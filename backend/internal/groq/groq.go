// Package groq wraps the Groq chat completions API.
//
// Groq's free tier caps tokens per minute — 8k on the gpt-oss models — and a
// single deployment session burns that within a handful of turns. Left
// unhandled, one mid-session 429 fails the whole deployment, so every call
// goes through Post, which waits out the rate limit and tries again.
package groq

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Endpoint is the OpenAI-compatible chat completions URL. It defaults to Groq
// but honors GROQ_BASE_URL (an OpenAI-compatible base such as
// http://host:1234/openai/v1) so a mock server or an alternative provider can be
// substituted — the smoke test and any future fallback provider rely on this.
// It is a package var, so a white-box test can also set it directly.
var Endpoint = func() string {
	if base := strings.TrimRight(os.Getenv("GROQ_BASE_URL"), "/"); base != "" {
		return base + "/chat/completions"
	}
	return "https://api.groq.com/openai/v1/chat/completions"
}()

const (
	maxAttempts = 5
	maxBackoff  = 60 * time.Second
)

// Groq reports the exact wait in the error message ("Please try again in
// 1.89s"), which is more accurate than any backoff we could guess.
var retryHintRe = regexp.MustCompile(`try again in ([0-9.]+)s`)

// output_parse_failed is a 400, but it reports a bad generation rather than a
// bad request: the model emitted something that was not a usable tool call.
// The same input usually succeeds on another attempt, so it is worth retrying
// where the other 4xx codes are not.
var parseFailedRe = regexp.MustCompile(`"code"\s*:\s*"output_parse_failed"`)

// Post sends one chat-completion request and returns the raw response body.
//
// Rate limits (429) and transient server errors (5xx) are retried; every other
// status is returned as an error on the first attempt, since retrying a bad
// request or a rejected API key only delays the failure. onRetry, when set, is
// called before each wait with a short reason so callers can surface the pause
// to the user accurately — a garbled generation is not a rate limit.
func Post(ctx context.Context, apiKey string, body []byte, onRetry func(wait time.Duration, attempt int, reason string)) ([]byte, error) {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, Endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("groq request: %w", err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read groq response: %w", readErr)
		}

		if resp.StatusCode == http.StatusOK {
			return respBody, nil
		}
		var reason string
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			reason = "rate limited"
		case resp.StatusCode >= 500:
			reason = fmt.Sprintf("server error %d", resp.StatusCode)
		case parseFailedRe.Match(respBody):
			reason = "returned unparseable output"
		default:
			return nil, fmt.Errorf("groq error %d: %s", resp.StatusCode, string(respBody))
		}

		lastErr = fmt.Errorf("groq error %d: %s", resp.StatusCode, string(respBody))
		if attempt == maxAttempts {
			break
		}

		wait := retryDelay(resp.Header.Get("Retry-After"), respBody, attempt)
		if onRetry != nil {
			onRetry(wait, attempt, reason)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	return nil, lastErr
}

// retryDelay prefers the wait the API asks for, falling back to exponential
// backoff. The small margin on top of Groq's figure keeps the retry from
// landing fractionally before the window reopens.
func retryDelay(retryAfter string, body []byte, attempt int) time.Duration {
	if secs, err := strconv.ParseFloat(retryAfter, 64); err == nil && secs > 0 {
		return capped(time.Duration(secs*float64(time.Second)) + 250*time.Millisecond)
	}
	if m := retryHintRe.FindSubmatch(body); m != nil {
		if secs, err := strconv.ParseFloat(string(m[1]), 64); err == nil && secs > 0 {
			return capped(time.Duration(secs*float64(time.Second)) + 250*time.Millisecond)
		}
	}
	return capped(time.Duration(1<<attempt) * time.Second)
}

func capped(d time.Duration) time.Duration {
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
