package auth

import (
	"slices"
	"strings"
	"testing"
)

func TestNormalizeEmails(t *testing.T) {
	got, err := NormalizeEmails([]string{" ada@example.com ", "Ada@Work.example", "me@localhost"})
	if err != nil || !slices.Equal(got, []string{"ada@example.com", "Ada@Work.example", "me@localhost"}) {
		t.Fatalf("NormalizeEmails = %v, %v", got, err)
	}
	for _, bad := range [][]string{
		nil,
		{"not an email"},
		{"Ada <ada@example.com>"},
		{"<ada@example.com>"},
		{"ada@example.com", "ADA@example.com"},
		{strings.Repeat("a", 250) + "@x.io"},
		slices.Repeat([]string{"x"}, MaxEmails+1),
	} {
		if _, err := NormalizeEmails(bad); err == nil {
			t.Errorf("NormalizeEmails(%q) accepted", bad)
		}
	}
}
