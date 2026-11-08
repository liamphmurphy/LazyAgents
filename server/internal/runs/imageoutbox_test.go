package runs

import (
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func TestImageOutboxTool(t *testing.T) {
	sid := "019e-thread"
	run := store.Run{HarnessSessionID: &sid}
	src := "/home/u/.codex/generated_images/" + sid + "/ig_1.png"
	cp := func(s, d string) ToolCheck {
		return ToolCheck{Tool: "Bash", Input: map[string]any{"command": "cp " + s + " " + d}}
	}
	cases := []struct {
		name string
		run  store.Run
		in   ToolCheck
		want bool
	}{
		{"valid", run, cp(src, ".lazyagents/outbox/a.png"), true},
		{"valid upper ext flags", run, cp("-f "+src, ".lazyagents/outbox/a.JPG"), true},
		{"mkdir", run, ToolCheck{Tool: "Bash", Input: map[string]any{"command": "mkdir -p .lazyagents/outbox"}}, true},
		{"glob", run, cp("/home/u/.codex/generated_images/*/*", ".lazyagents/outbox/."), false},
		{"glob in dst name", run, cp(src, ".lazyagents/outbox/*.png"), false},
		{"brace", run, cp("/home/u/.codex/generated_images/"+sid+"/{a,b}.png", ".lazyagents/outbox/a.png"), false},
		{"tilde", run, cp("~/.codex/generated_images/"+sid+"/a.png", ".lazyagents/outbox/a.png"), false},
		{"dot dst", run, cp(src, ".lazyagents/outbox/."), false},
		{"dotfile dst", run, cp(src, ".lazyagents/outbox/.hidden.png"), false},
		{"bad ext", run, cp(src, ".lazyagents/outbox/a.sh"), false},
		{"dotfile src", run, cp("/home/u/.codex/generated_images/"+sid+"/.x.png", ".lazyagents/outbox/a.png"), false},
		{"wrong thread", run, cp("/home/u/.codex/generated_images/other/ig.png", ".lazyagents/outbox/a.png"), false},
		{"wrong home dir", run, cp("/home/u/.ssh/generated_images/"+sid+"/ig.png", ".lazyagents/outbox/a.png"), false},
		{"nil session", store.Run{}, cp(src, ".lazyagents/outbox/a.png"), false},
		{"traversal", run, cp("/home/u/../../etc/.codex/generated_images/"+sid+"/x.png", ".lazyagents/outbox/a.png"), false},
		{"passwd", run, cp("/etc/passwd", ".lazyagents/outbox/a.png"), false},
		{"outside dst", run, cp(src, "/tmp/a.png"), false},
		{"subdir dst", run, cp(src, ".lazyagents/outbox/sub/a.png"), false},
		{"chain", run, cp(src, ".lazyagents/outbox/a.png && id"), false},
		{"redirect", run, cp(src, ".lazyagents/outbox/a.png > /tmp/x"), false},
		{"env var", run, cp(src, "$HOME/.lazyagents/outbox/a.png"), false},
		{"recursive", run, cp("-r "+src, ".lazyagents/outbox/a.png"), false},
		{"rm", run, ToolCheck{Tool: "Bash", Input: map[string]any{"command": "rm -rf .lazyagents/outbox"}}, false},
		{"Write tool", run, ToolCheck{Tool: "Write"}, false},
		{"nil session tool", store.Run{}, ToolCheck{Tool: "image_generation"}, true},
	}
	for _, c := range cases {
		if got := imageOutboxTool(c.run, c.in); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
