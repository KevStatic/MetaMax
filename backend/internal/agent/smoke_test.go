//go:build smoke

// Package agent's end-to-end smoke test. It drives the real agent loop and the
// real container manager against a live Docker-in-Docker daemon, but replaces
// the Groq API with a local mock that scripts a deterministic deploy — so it
// never touches the LLM quota and never depends on model nondeterminism.
//
// It is guarded by the `smoke` build tag, so the normal hermetic unit suite
// (`go test ./...`) neither compiles nor runs it. To run it you need a reachable
// Docker daemon and a writable, dind-shared /vm-storage:
//
//	DOCKER_HOST=tcp://dind:2375 go test -tags smoke ./internal/agent/...
//
// The cleanest way to satisfy the environment is from inside the compose stack,
// which already shares /vm-storage between the backend and dind.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shreejaykurhade/MetaMax/backend/internal/container"
	"github.com/shreejaykurhade/MetaMax/backend/internal/groq"
)

const smokeMarker = "METAMAX_SMOKE_OK"

// mockGroq scripts the deploy as a fixed sequence of tool calls, keyed off how
// many tool results the agent has already sent back. Steps that operate on the
// container read its id out of the create_container result already in the
// conversation, exactly as the real model would.
func mockGroq(t *testing.T, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)

		containerID := ""
		toolResults := 0
		for _, m := range req.Messages {
			if m.Role != "tool" {
				continue
			}
			toolResults++
			var obj map[string]any
			if json.Unmarshal([]byte(m.Content), &obj) == nil {
				if id, ok := obj["ID"].(string); ok && id != "" {
					if _, isContainer := obj["StoragePath"]; isContainer {
						containerID = id
					}
				}
			}
		}

		if toolResults >= 1 && containerID == "" {
			t.Errorf("mock: no container id available at step %d", toolResults)
			writeFinal(w)
			return
		}

		switch toolResults {
		case 0:
			writeToolCall(w, "create_container", map[string]any{
				"name": name, "image": "python:3.12-slim", "ports": []string{"8080:8080"},
			})
		case 1:
			writeToolCall(w, "write_file", map[string]any{
				"container_id": containerID, "path": "/app/index.html", "content": smokeMarker,
			})
		case 2:
			writeToolCall(w, "start_process", map[string]any{
				"container_id": containerID, "command": "python3 -m http.server 8080", "work_dir": "/app",
			})
		case 3:
			writeToolCall(w, "run_command", map[string]any{
				"container_id": containerID,
				"command":      `sleep 1; python3 -c 'import urllib.request; print(urllib.request.urlopen("http://localhost:8080/").read().decode())'`,
				"work_dir":     "/app",
			})
		case 4:
			writeToolCall(w, "health_check", map[string]any{"container_id": containerID})
		case 5:
			writeToolCall(w, "destroy_container", map[string]any{"container_id": containerID})
		default:
			writeFinal(w)
		}
	}
}

func writeToolCall(w http.ResponseWriter, name string, args map[string]any) {
	argsJSON, _ := json.Marshal(args)
	resp := map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []map[string]any{{
					"id":       "call_" + name,
					"type":     "function",
					"function": map[string]any{"name": name, "arguments": string(argsJSON)},
				}},
			},
			"finish_reason": "tool_calls",
		}},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func writeFinal(w http.ResponseWriter) {
	resp := map[string]any{
		"choices": []map[string]any{{
			"message":       map[string]any{"role": "assistant", "content": "Smoke deployment complete."},
			"finish_reason": "stop",
		}},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func TestSmokeDeploy(t *testing.T) {
	mgr, err := container.NewManager(os.Getenv("DOCKER_HOST"))
	if err != nil {
		t.Fatalf("container manager: %v", err)
	}

	name := fmt.Sprintf("smoke-%d", time.Now().UnixNano()%1_000_000)
	mock := httptest.NewServer(mockGroq(t, name))
	defer mock.Close()

	prevEndpoint := groq.Endpoint
	groq.Endpoint = mock.URL
	defer func() { groq.Endpoint = prevEndpoint }()

	sess := NewSession(SessionConfig{
		ID:         name,
		TeamID:     "smoke",
		Manager:    mgr,
		GroqAPIKey: "test-key",
		Model:      "mock",
		Autopilot:  true,
	})
	go func() {
		for range sess.Events() { // drain so emit() never backs up
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := sess.Run(ctx, "Deploy a smoke workload that serves a static page on port 8080."); err != nil {
		t.Fatalf("session run: %v", err)
	}

	if sess.State != StateCompleted {
		t.Errorf("state = %q, want %q", sess.State, StateCompleted)
	}
	if sess.MerkleRoot == "" {
		t.Errorf("no Merkle root computed")
	}

	// The workload actually served its page: the in-container fetch printed the
	// marker back through run_command's captured output.
	served := false
	for _, a := range sess.Actions {
		if a.Tool != "run_command" {
			continue
		}
		if m, ok := a.Result.(map[string]string); ok && strings.Contains(m["output"], smokeMarker) {
			served = true
		}
	}
	if !served {
		t.Errorf("deployed workload did not serve %q", smokeMarker)
	}
}
