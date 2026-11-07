package wiki

import (
	"reflect"
	"testing"
)

func TestParseADR(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       ADRStatus
	}{
		{"none", "# ADR\n\nNo status here.\n", ADRStatus{State: ADRNone}},
		{"accepted", "Status: Accepted (2026-11-01). Ticket: [[lazy-4]].\n", ADRStatus{Status: "Accepted", State: ADRAccepted, Date: "2026-11-01", Ticket: "LAZY-4"}},
		{"bold proposed", "**Status:** Proposed\n", ADRStatus{Status: "Proposed", State: ADRProposed}},
		{"superseded", "Status: Superseded by [[ADR-014]].\n", ADRStatus{Status: "Superseded by [[ADR-014]]", State: ADRSuperseded}},
		{"open questions without a list", "Status: Accepted, with open questions\n", ADRStatus{Status: "Accepted", State: ADRAccepted, HasOpenQuestions: true}},
		{"open questions", "Status: Accepted, with open questions (2026-11-06).\n\n## Open questions\n\n- One\n  - nested detail\n- Two\n  continued\n\n```\n- not this\n```\n\n## Next\n\n- Not a question\n",
			ADRStatus{Status: "Accepted", State: ADRAccepted, Date: "2026-11-06", OpenQuestions: []string{"One - nested detail", "Two continued"}, HasOpenQuestions: true}},
		{"other", "Status: Under discussion\n", ADRStatus{Status: "Under discussion", State: ADROther}},
	} {
		if got := ParseADR(c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
