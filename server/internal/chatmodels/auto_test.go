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

func TestAutoModel(t *testing.T) {
	ws := uuid.New()
	if got := AutoModel(ws, "claude-code", "default", "xhigh", true); got != "default" {
		t.Fatalf("unlisted catalog gave %q", got)
	}
	rememberOffered(ws, []Harness{
		{Harness: "claude-code", Models: []Model{
			{ID: "default", Label: "Default (recommended)", Default: true},
			{ID: "opus", Label: "Opus 5.5"},
			{ID: "fable", Label: "Fable 5.2"},
			{ID: "haiku", Label: "Haiku 5.5"},
		}},
		{Harness: "codex", Models: []Model{
			{ID: "gpt-6-codex", Default: true},
			{ID: "gpt-6-codex-mini"},
		}},
	})
	for _, c := range []struct {
		name, harness, current, level string
		fresh                         bool
		want                          string
	}{
		{"fresh light turn starts small", "claude-code", "default", "low", true, "haiku"},
		{"fresh ordinary turn starts on the default", "claude-code", "haiku", "high", true, "default"},
		{"fresh deep turn starts large", "claude-code", "default", "xhigh", true, "fable"},
		{"ongoing never steps down", "claude-code", "fable", "low", false, "fable"},
		{"ongoing default stays for a light turn", "claude-code", "default", "low", false, "default"},
		{"ongoing small moves up", "claude-code", "haiku", "medium", false, "default"},
		{"ongoing moves up to large", "claude-code", "default", "xhigh", false, "fable"},
		{"a named standard model counts as standard", "claude-code", "opus", "high", false, "opus"},
		{"missing large tier keeps the default", "codex", "gpt-6-codex", "xhigh", false, "gpt-6-codex"},
		{"missing large tier, small moves to the default", "codex", "gpt-6-codex-mini", "xhigh", false, "gpt-6-codex"},
		{"codex mini is small", "codex", "gpt-6-codex", "low", true, "gpt-6-codex-mini"},
	} {
		if got := AutoModel(ws, c.harness, c.current, c.level, c.fresh); got != c.want {
			t.Errorf("%s: AutoModel(%s, %s, fresh=%v) = %q, want %q", c.name, c.current, c.level, c.fresh, got, c.want)
		}
	}
}
