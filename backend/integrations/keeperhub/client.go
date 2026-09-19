// Package keeperhub provides an interface and implementations for KeeperHub
// job registration.  The agent calls RegisterJob at session completion so that
// an off-chain keeper can trigger the on-chain attestation submission.
package keeperhub

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Job is the payload submitted to KeeperHub.
type Job struct {
	Name        string `json:"name"`
	SessionID   string `json:"session_id"`
	Payload     any    `json:"payload"`
	CallbackURL string `json:"callback_url,omitempty"`
}

// registerJobRequest is the request body sent to the KeeperHub API.
type registerJobRequest struct {
	Job       Job    `json:"job"`
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
}

// registerJobResponse is the KeeperHub API response for job registration.
type registerJobResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// Client is the interface used by the agent to manage keeper jobs.
type Client interface {
	RegisterJob(ctx context.Context, job Job) (string, error)
	CancelJob(ctx context.Context, jobID string) error
}

// NoopClient is a Client that silently discards all operations.
type NoopClient struct{}

func (NoopClient) RegisterJob(_ context.Context, _ Job) (string, error) { return "", nil }
func (NoopClient) CancelJob(_ context.Context, _ string) error           { return nil }

// httpClient is a KeeperHub HTTP client with HMAC-SHA256 request signing.
type httpClient struct {
	endpoint   string
	privateKey string
	http       *http.Client
}

// RegisterJob submits a job to KeeperHub. The request body is signed with
// HMAC-SHA256 using the configured private key.
func (c *httpClient) RegisterJob(ctx context.Context, job Job) (string, error) {
	ts := time.Now().Unix()
	// Create a unique nonce from session ID + timestamp
	nonce := fmt.Sprintf("%s-%d", job.SessionID, ts)

	reqBody := registerJobRequest{
		Job:       job,
		Timestamp: ts,
		Nonce:     nonce,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("keeperhub: marshal request: %w", err)
	}

	// Compute HMAC-SHA256 signature over the raw JSON body
	sig := computeHMAC(bodyBytes, c.privateKey)

	url := strings.TrimRight(c.endpoint, "/") + "/api/v1/jobs"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("keeperhub: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "HMAC "+sig)
	httpReq.Header.Set("X-Timestamp", fmt.Sprintf("%d", ts))
	httpReq.Header.Set("X-Nonce", nonce)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("keeperhub: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("keeperhub: server error %d: %s", resp.StatusCode, raw)
	}

	var result registerJobResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		// Some implementations return just the job ID as a plain string
		return strings.Trim(string(raw), `"`), nil
	}
	return result.JobID, nil
}

// CancelJob cancels a previously registered job.
func (c *httpClient) CancelJob(ctx context.Context, jobID string) error {
	url := strings.TrimRight(c.endpoint, "/") + "/api/v1/jobs/" + jobID

	body := []byte(`{"status":"cancelled"}`)
	sig := computeHMAC(body, c.privateKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("keeperhub: build cancel request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "HMAC "+sig)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("keeperhub: cancel request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("keeperhub: cancel error %d", resp.StatusCode)
	}
	return nil
}

// computeHMAC computes a hex-encoded HMAC-SHA256 over body using key.
func computeHMAC(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// New returns a KeeperHub client if credentials are provided, otherwise a NoopClient.
func New(endpoint, privateKey string) (Client, error) {
	if endpoint == "" || privateKey == "" {
		return NoopClient{}, nil
	}
	return &httpClient{
		endpoint:   endpoint,
		privateKey: privateKey,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, nil
}
