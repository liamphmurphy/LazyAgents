package runner

import (
	"strings"
	"testing"
)

func TestParseChatSuggestions(t *testing.T) {
	entry := func(title, prompt, icon string) string {
		return `{"title":"` + title + `","prompt":"` + prompt + `","icon":"` + icon + `"}`
	}
	four := strings.Repeat(entry("Title", "A prompt", "dice")+",", 3) + entry("Last", "Final prompt", "bad")
	got, err := parseChatSuggestions("```json\n[" + four + "]\n```")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3].Icon != "msg" || got[0].Icon != "dice" {
		t.Fatalf("parsed %+v", got)
	}
	longTitle := entry(strings.Repeat("x", chatSuggestionTitleSize+1), "A prompt", "dice")
	for name, answer := range map[string]string{
		"prose":     "I can't help with that.",
		"malformed": "[{]",
		"too few":   "[" + entry("Title", "A prompt", "dice") + "]",
		"too long":  "[" + strings.Repeat(entry("Title", "A prompt", "dice")+",", 3) + longTitle + "]",
		"blank":     "[" + strings.Repeat(entry("Title", "A prompt", "dice")+",", 3) + entry(" ", "A prompt", "dice") + "]",
	} {
		if out, err := parseChatSuggestions(answer); err == nil {
			t.Errorf("%s: accepted %+v", name, out)
		} else if strings.Contains(err.Error(), "can't help") {
			t.Errorf("%s: error repeats the model's text: %v", name, err)
		}
	}
	five := strings.Repeat(entry("Title", "A prompt", "dice")+",", 4) + entry("Fifth", "A prompt", "dice")
	if got, err := parseChatSuggestions("[" + five + "]"); err != nil || len(got) != 4 {
		t.Fatalf("five entries: %d %v", len(got), err)
	}
}
