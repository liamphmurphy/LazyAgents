package auth

import (
	"slices"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/store"
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

func TestCommitEmail(t *testing.T) {
	u := store.User{Email: "ada@example.com", ExtraEmails: []string{"Ada@Work.example"}}
	pick := func(s string) *string { return &s }
	for _, c := range []struct {
		picked *string
		want   string
	}{
		{nil, "ada@example.com"},
		{pick("ada@work.example"), "Ada@Work.example"},
		{pick(" ada@example.com "), "ada@example.com"},
		// No longer one of theirs: back to the primary.
		{pick("old@example.com"), "ada@example.com"},
	} {
		if got := CommitEmail(u, c.picked); got != c.want {
			t.Errorf("CommitEmail(%v) = %q, want %q", c.picked, got, c.want)
		}
	}
}
