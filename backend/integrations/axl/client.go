// Package axl provides an interface and implementations for AXL (Gensyn)
// P2P messaging. The AXL node exposes a local HTTP API on port 9002.
// Messages are sent via POST /send (with X-Destination-Peer-Id header) and
// received via GET /recv (long-poll).
//
// Configuration:
//   AXL_ENDPOINT   — local node API URL, e.g. http://127.0.0.1:9002
//   AXL_PEER_ID    — destination peer's 64-char hex public key (for publishing)
//
// Topic filtering is done by embedding the topic in the JSON message envelope.
package axl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the interface used by the agent to publish/subscribe events.
type Client interface {
	// Publish sends msg to the given topic.
	Publish(ctx context.Context, topic string, msg []byte) error
	// Subscribe registers handler for messages on topic.
	Subscribe(ctx context.Context, topic string, handler func(msg []byte)) error
}

// NoopClient is a Client that silently discards all messages.
type NoopClient struct{}

func (NoopClient) Publish(_ context.Context, _ string, _ []byte) error        { return nil }
func (NoopClient) Subscribe(_ context.Context, _ string, _ func([]byte)) error { return nil }

// envelope is the JSON wrapper sent over the AXL wire so the receiver can
// filter by topic without inspecting the opaque msg payload.
type envelope struct {
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
}

// recvResponse is returned by the AXL node's GET /recv endpoint.
type recvResponse struct {
	Body       []byte `json:"body"`       // raw message bytes
	FromPeerID string `json:"from_peer_id"`
}

// httpClient talks to a locally-running AXL node.
//   endpoint — e.g. "http://127.0.0.1:9002"
//   peerID   — destination peer's 64-char hex ed25519 public key
type httpClient struct {
	endpoint string
	peerID   string
	http     *http.Client
}

// Publish sends msg wrapped in a topic envelope to the configured destination peer.
func (c *httpClient) Publish(ctx context.Context, topic string, msg []byte) error {
	env := envelope{Topic: topic, Payload: msg}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("axl Publish: marshal envelope: %w", err)
	}
	url := strings.TrimRight(c.endpoint, "/") + "/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("axl Publish: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Destination-Peer-Id", c.peerID)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("axl Publish: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("axl Publish: node returned %d: %s", resp.StatusCode, raw)
	}
	return nil
}

// Subscribe polls GET /recv on the local AXL node in a background goroutine.
// Only envelopes matching topic are passed to handler. Exits when ctx is cancelled.
func (c *httpClient) Subscribe(ctx context.Context, topic string, handler func(msg []byte)) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			url := strings.TrimRight(c.endpoint, "/") + "/recv"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			resp, err := c.http.Do(req)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()

			if resp.StatusCode == http.StatusNoContent || len(raw) == 0 {
				// No message available yet; short backoff.
				time.Sleep(250 * time.Millisecond)
				continue
			}

			var result recvResponse
			if err := json.Unmarshal(raw, &result); err != nil {
				time.Sleep(250 * time.Millisecond)
				continue
			}

			var env envelope
			if err := json.Unmarshal(result.Body, &env); err == nil {
				if env.Topic == topic {
					handler(env.Payload)
				}
			}
		}
	}()
	return nil
}

// New returns an AXL client if credentials are provided, otherwise a NoopClient.
// endpoint is the local AXL node API URL (e.g. http://127.0.0.1:9002).
// peerID is the destination peer's 64-char hex public key (used for Publish).
func New(endpoint, peerID string) (Client, error) {
	if endpoint == "" || peerID == "" {
		return NoopClient{}, nil
	}
	return &httpClient{
		endpoint: endpoint,
		peerID:   peerID,
		http:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// SessionTopic returns the canonical topic name for a session.
func SessionTopic(sessionID string) string {
	return "comput3.session." + sessionID
}
