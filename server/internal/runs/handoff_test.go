package runs

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func hist(n int, bodyLen int) []HandoffMessage {
	// newest first, seq n..1
	var out []HandoffMessage
	for i := n; i >= 1; i-- {
		out = append(out, HandoffMessage{Seq: int64(i), Role: "user", Body: strings.Repeat(string(rune('a'+i%26)), bodyLen)})
	}
	return out
}

func TestRenderHandoffKeepsNewestAtBudgetBoundary(t *testing.T) {
	h := hist(5, 100)
	entry := utf8.RuneCountInString(handoffEntry(h[0], h[0].Body))
	// Room for exactly three entries after the header (worst-case omitted=5).
	budget := utf8.RuneCountInString(handoffHeader(5)) + 3*entry
	got := RenderHandoff(h, 5, budget, "NEW")
	if !strings.Contains(got, "2 earlier message(s) were omitted") {
		t.Fatalf("omitted count: %s", got)
	}
	for _, kept := range []string{"[3] ", "[4] ", "[5] "} {
		if !strings.Contains(got, kept) {
			t.Errorf("missing newest %s", kept)
		}
	}
	for _, dropped := range []string{"[1] ", "[2] "} {
		if strings.Contains(got, dropped) {
			t.Errorf("kept oldest %s", dropped)
		}
	}
	if strings.Index(got, "[3] ") > strings.Index(got, "[5] ") {
		t.Error("oldest-first order broken")
	}
	if !strings.HasSuffix(got, handoffFooter+"NEW") {
		t.Error("new prompt must follow history separately")
	}
	// One char less drops another message.
	got = RenderHandoff(h, 5, budget-1, "NEW")
	if !strings.Contains(got, "3 earlier message(s) were omitted") || strings.Contains(got, "[3] ") {
		t.Fatalf("below boundary: %s", got)
	}
	// Everything fits: no omission note.
	got = RenderHandoff(h, 5, 0, "NEW")
	if strings.Contains(got, "omitted") || !strings.Contains(got, "[1] ") {
		t.Fatalf("full history: %s", got)
	}
}

func TestRenderHandoffCountsRowsBeyondQuery(t *testing.T) {
	got := RenderHandoff(hist(3, 10), 50, 0, "NEW")
	if !strings.Contains(got, "47 earlier message(s) were omitted") {
		t.Fatal(got)
	}
}

func TestRenderHandoffClipsOversizedNewestMessage(t *testing.T) {
	h := []HandoffMessage{{Seq: 2, Role: "assistant", Harness: "codex", Model: "m", Body: strings.Repeat("x", 5000)}, {Seq: 1, Role: "user", Body: "old"}}
	got := RenderHandoff(h, 2, 500, "NEW")
	hist := strings.TrimSuffix(got, handoffFooter+"NEW")
	if utf8.RuneCountInString(hist) > 500 || !strings.Contains(got, "clipped") || !strings.Contains(got, "1 earlier") || strings.Contains(got, "[1] ") {
		t.Fatalf("len=%d %s", utf8.RuneCountInString(hist), got)
	}
}
