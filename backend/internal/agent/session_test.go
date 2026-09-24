package agent

import (
	"fmt"
	"strings"
	"testing"
)

func TestTruncateForModel(t *testing.T) {
	// Anything at or under the cap passes through untouched.
	short := "small output"
	if got := truncateForModel(short); got != short {
		t.Errorf("short string changed: got %q", got)
	}
	atCap := strings.Repeat("a", maxToolResultChars)
	if got := truncateForModel(atCap); got != atCap {
		t.Errorf("string at cap changed (len %d)", len(got))
	}

	// Over the cap collapses to head + tail with an elision marker, and both
	// ends survive so the model still sees the command and its final status.
	big := strings.Repeat("H", toolResultHead) + strings.Repeat("M", 5000) + strings.Repeat("T", toolResultTail)
	got := truncateForModel(big)
	if len(got) >= len(big) {
		t.Fatalf("result not smaller: %d >= %d", len(got), len(big))
	}
	if !strings.HasPrefix(got, strings.Repeat("H", toolResultHead)) {
		t.Errorf("head not preserved")
	}
	if !strings.HasSuffix(got, strings.Repeat("T", toolResultTail)) {
		t.Errorf("tail not preserved")
	}
	if !strings.Contains(got, "bytes elided") {
		t.Errorf("missing elision marker: %q", got)
	}
}

func TestCompactToolHistory(t *testing.T) {
	messages := []groqMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "go"},
	}
	total := keepVerbatimResults + 4 // more results than the keep window
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("c%d", i)
		messages = append(messages,
			groqMessage{Role: "assistant", ToolCalls: []toolCall{{ID: id}}},
			groqMessage{Role: "tool", ToolCallID: id, Content: fmt.Sprintf("result-%d", i)},
		)
	}

	compactToolHistory(messages)

	var toolMsgs []groqMessage
	for _, m := range messages {
		if m.Role == "tool" {
			toolMsgs = append(toolMsgs, m)
		}
	}
	if len(toolMsgs) != total {
		t.Fatalf("expected %d tool messages, found %d", total, len(toolMsgs))
	}
	// Only the most recent keepVerbatimResults stay full; the rest are stubbed.
	for n, m := range toolMsgs {
		wantStub := n < total-keepVerbatimResults
		gotStub := m.Content == toolResultStub
		if wantStub != gotStub {
			t.Errorf("tool result %d: wantStub=%v gotStub=%v (content %q)", n, wantStub, gotStub, m.Content)
		}
	}
	// Non-tool messages are never touched.
	if messages[0].Content != "sys" || messages[1].Content != "go" {
		t.Errorf("non-tool messages were modified")
	}

	// Idempotent: a second pass changes nothing.
	before := make([]string, len(messages))
	for i, m := range messages {
		before[i] = m.Content
	}
	compactToolHistory(messages)
	for i, m := range messages {
		if m.Content != before[i] {
			t.Errorf("compaction not idempotent at index %d", i)
		}
	}
}
