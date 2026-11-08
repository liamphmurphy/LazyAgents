package runs

import (
	"strings"
)

// The Library keeps a person's assistant-chat keepsakes for good: for now,
// every image a persona generates is added when its reply is stored.

// MaxLibraryTitleRunes matches library_items.title.
const MaxLibraryTitleRunes = 300

// LibraryImageTitle names a generated image after the chat it came from, so
// it stays recognisable after the chat is gone.
func LibraryImageTitle(chatTitle string) string {
	t := strings.TrimSpace(chatTitle)
	if t == "" {
		return "Generated image"
	}
	if r := []rune(t); len(r) > MaxLibraryTitleRunes {
		t = string(r[:MaxLibraryTitleRunes])
	}
	return t
}
