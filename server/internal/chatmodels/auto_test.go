package chatmodels

import (
	"testing"

	"github.com/google/uuid"
)

func TestAutoLevel(t *testing.T) {
	for text, want := range map[string]string{
		"thanks!":                           "low",
		"what's up?":                        "low",
		"how long did that run take?":       "medium",
		"fix the failing test":              "high",
		"look at chat.go":                   "high",
		"do a thorough audit of the runner": "xhigh",
		"thanks, that refactor looks great": "low",
		"thanks for the fix":                "low",
		"does that make sense":              "low",
		"the test passed":                   "low",
		"cool, add a test":                  "high",
		"thanks, fix that too":              "high",
		"thanks, now refactor the runner":   "xhigh",
		"cool, refactor it":                 "xhigh",
		"thanks, is chat.go ready?":         "high",
		"why is the test failing?":          "high",
		"can you make it faster":            "high",
		"make me a battle map":              "high",
	} {
		if got := AutoLevel(text); got != want {
			t.Errorf("AutoLevel(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestAutoEffort(t *testing.T) {
	ws := uuid.New()
	if got := AutoEffort(ws, "codex", "any", "high"); got != "" {
		t.Fatalf("unlisted catalog gave %q", got)
	}
	rememberOffered(ws, []Harness{{Harness: "codex", Models: []Model{
		{ID: "big", Default: true, Efforts: []string{"low", "medium", "high", "xhigh"}},
		{ID: "mini", Efforts: []string{"medium", "high"}},
		{ID: "none", Efforts: []string{}},
	}}})
	for _, c := range []struct{ model, level, want string }{
		{"big", "low", "low"},
		{"big", "xhigh", "xhigh"},
		{"mini", "low", "medium"},
		{"mini", "xhigh", "high"},
		{"none", "high", ""},
		{"alias", "medium", "medium"},
	} {
		if got := AutoEffort(ws, "codex", c.model, c.level); got != c.want {
			t.Errorf("AutoEffort(%s, %s) = %q, want %q", c.model, c.level, got, c.want)
		}
	}
}
