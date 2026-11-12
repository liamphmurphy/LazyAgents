package docs

import (
	"strings"
	"testing"
)

func TestWikiPath(t *testing.T) {
	for _, tc := range []struct{ requested, title, want string }{
		{"", "Rollback runbook", "Rollback runbook"},
		{"  Payments/Rollback runbook ", "ignored", "Payments/Rollback runbook"},
		{"", "Q3 / Q4 plan [draft] #1", "Q3 Q4 plan draft 1"},
		{"", "  ", UntitledTitle},
		{"", strings.Repeat("é", 400), strings.Repeat("é", 300)},
	} {
		if got := WikiPath(tc.requested, tc.title); got != tc.want {
			t.Errorf("WikiPath(%q, %q) = %q, want %q", tc.requested, tc.title, got, tc.want)
		}
	}
}

func TestWikiBodyNamesItsSource(t *testing.T) {
	got := WikiBody("# Plan\n\nText\n\n", "Plan", "2026-11-12")
	want := "# Plan\n\nText\n\n---\n\n_Promoted from the Doc “Plan” on 2026-11-12._\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
