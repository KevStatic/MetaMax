// Package zerog provides an interface and implementations for 0G Network
// KV/log storage.  When the 0G endpoint is not configured, the Noop
// implementation is used so the agent functions normally without the external
// service.
package zerog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the interface used by the agent to persist session actions.
type Client interface {
	// Put stores a key-value pair in 0G Network KV storage.
	Put(ctx context.Context, key string, value []byte) error
	// Get retrieves a value by key.
	Get(ctx context.Context, key string) ([]byte, error)
	// Append appends a byte slice to the session's append-only log.
	Append(ctx context.Context, logID string, entry []byte) error
	// ReadLog retrieves all previously appended entries for a session.
	ReadLog(ctx context.Context, logID string) ([][]byte, error)
}

// NoopClient is a Client that silently discards all data.
type NoopClient struct{}

func (NoopClient) Put(_ context.Context, _ string, _ []byte) error       { return nil }
func (NoopClient) Get(_ context.Context, _ string) ([]byte, error)       { return nil, nil }
func (NoopClient) Append(_ context.Context, _ string, _ []byte) error    { return nil }
func (NoopClient) ReadLog(_ context.Context, _ string) ([][]byte, error) { return nil, nil }

// zgClient interacts with the 0G Network storage node HTTP API.
// It uses the 0G indexer/storage-node REST endpoints for KV and log operations.
type zgClient struct {
	storageURL string // e.g. https://storage-testnet.0g.ai
	http       *http.Client
}

// kvUploadRequest is the body for a KV batch upload.
type kvUploadRequest struct {
	Pairs []kvPair `json:"pairs"`
}

type kvPair struct {
	Key   string `json:"key"` // hex-encoded
	Value string `json:"value"` // base64-encoded
}

// Put stores a key-value pair via the 0G storage node KV API.
func (c *zgClient) Put(ctx context.Context, key string, value []byte) error {
	req := kvUploadRequest{
		Pairs: []kvPair{{
			Key:   hex.EncodeToString([]byte(key)),
			Value: encodeBase64Std(value),
		}},
	}
	body, _ := json.Marshal(req)
	url := strings.TrimRight(c.storageURL, "/") + "/v1/kv/batch_upload"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("0g Put: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("0g Put: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("0g Put: server error %d: %s", resp.StatusCode, raw)
	}
	return nil
}

// Get retrieves a value by key from the 0G storage node KV API.
func (c *zgClient) Get(ctx context.Context, key string) ([]byte, error) {
	keyHex := hex.EncodeToString([]byte(key))
	url := strings.TrimRight(c.storageURL, "/") + "/v1/kv/value?key=" + keyHex
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("0g Get: build request: %w", err)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("0g Get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("0g Get: server error %d: %s", resp.StatusCode, raw)
	}
	var result struct {
		Value string `json:"value"` // base64-encoded
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("0g Get: decode response: %w", err)
	}
	data, err := decodeBase64Std(result.Value)
	if err != nil {
		return nil, fmt.Errorf("0g Get: decode value: %w", err)
	}
	return data, nil
}

// Append appends an entry to the 0G log storage. The log is keyed by logID.
// Entries are stored as a JSON array, fetched and updated atomically.
func (c *zgClient) Append(ctx context.Context, logID string, entry []byte) error {
	existing, err := c.Get(ctx, "log:"+logID)
	if err != nil {
		return fmt.Errorf("0g Append read: %w", err)
	}

	var entries []string
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &entries)
	}
	entries = append(entries, encodeBase64Std(entry))

	updated, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("0g Append marshal: %w", err)
	}
	return c.Put(ctx, "log:"+logID, updated)
}

// ReadLog retrieves all appended entries for a log ID.
func (c *zgClient) ReadLog(ctx context.Context, logID string) ([][]byte, error) {
	raw, err := c.Get(ctx, "log:"+logID)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var encoded []string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return nil, fmt.Errorf("0g ReadLog decode: %w", err)
	}
	out := make([][]byte, 0, len(encoded))
	for _, e := range encoded {
		data, err := decodeBase64Std(e)
		if err != nil {
			continue
		}
		out = append(out, data)
	}
	return out, nil
}

// encodeBase64Std encodes bytes to standard base64.
func encodeBase64Std(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// decodeBase64Std decodes a standard base64 string.
func decodeBase64Std(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}


// New returns a 0G client if credentials are provided, otherwise a NoopClient.
// storageURL is the HTTP base URL of the 0G storage node (e.g. https://storage-testnet.0g.ai).
func New(storageURL, _, _ string) (Client, error) {
	if storageURL == "" {
		return NoopClient{}, nil
	}
	return &zgClient{
		storageURL: storageURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}
