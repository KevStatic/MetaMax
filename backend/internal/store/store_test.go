package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripJSONNul(t *testing.T) {
	nul := string([]byte{0})   // a real NUL byte, with no NUL in this source
	bs := string([]byte{0x5c}) // a single backslash, with no backslash in this source
	esc := bs + "u0000"        // the six characters a JSON NUL escape is written as

	// A real NUL in captured output marshals to the escape and must be removed.
	withNul, _ := json.Marshal(map[string]string{"output": "ok" + nul + "done"})
	if !strings.Contains(string(withNul), esc) {
		t.Fatalf("precondition: a NUL should marshal to its escape, got %s", withNul)
	}
	got := string(stripJSONNul(withNul))
	if strings.Contains(got, esc) {
		t.Errorf("NUL escape not removed: %s", got)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("result is not valid JSON: %v (%s)", err, got)
	}
	if out["output"] != "okdone" {
		t.Errorf("surrounding content lost: want %q, got %q", "okdone", out["output"])
	}

	// A six-byte escape that merely appears as text marshals with a doubled
	// backslash and must NOT be altered. A naive replace would corrupt it.
	literalText := "pre" + esc + "post"
	literal, _ := json.Marshal(map[string]string{"output": literalText})
	if gotLit := string(stripJSONNul(literal)); gotLit != string(literal) {
		t.Errorf("literal escape altered: before=%s after=%s", literal, gotLit)
	}

	// Clean input passes through unchanged.
	clean, _ := json.Marshal(map[string]string{"output": "nothing special"})
	if string(stripJSONNul(clean)) != string(clean) {
		t.Errorf("clean input was modified")
	}

	// Mixed: a real NUL and a text escape in the same document.
	mixed, _ := json.Marshal(map[string]string{"a": "x" + nul + "y", "b": "lit" + esc + "eral"})
	gotMixed := string(stripJSONNul(mixed))
	var mout map[string]string
	if err := json.Unmarshal([]byte(gotMixed), &mout); err != nil {
		t.Fatalf("mixed result is not valid JSON: %v (%s)", err, gotMixed)
	}
	if mout["a"] != "xy" {
		t.Errorf("mixed: real NUL not stripped, got a=%q", mout["a"])
	}
	if mout["b"] != "lit"+esc+"eral" {
		t.Errorf("mixed: text escape corrupted, got b=%q", mout["b"])
	}
}
