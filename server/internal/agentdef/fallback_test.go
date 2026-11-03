package agentdef

import (
	"encoding/json"
	"testing"
)

func TestFallbacks(t *testing.T) {
	s := base()
	s.Fallbacks = []Fallback{{Harness: HarnessCodex, Model: " gpt-fallback ", HarnessOptions: map[string]string{"sandbox": "workspace-write"}}}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	r := Resolve(s, nil)
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var restored Resolved
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Fallbacks) != 1 || restored.Fallbacks[0].Model != "gpt-fallback" || restored.Fallbacks[0].HarnessOptions["sandbox"] != "workspace-write" {
		t.Fatalf("lost fallback: %+v", restored)
	}
	r = Resolve(s, &Override{Harness: HarnessCodex, Model: "gpt-fallback"})
	if len(r.Fallbacks) != 0 {
		t.Fatal("override must not retry its own target")
	}
	for _, f := range []Fallback{{Harness: "unknown", Model: "test"}, {Harness: HarnessCodex}, {Harness: s.Harness, Model: s.Model}} {
		s.Fallbacks = []Fallback{f}
		if s.Normalize() == nil {
			t.Fatalf("accepted invalid fallback %+v", f)
		}
	}
}
