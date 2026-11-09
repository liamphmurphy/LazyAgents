package runs

import (
	"strings"
	"testing"
)

func TestResearchInputDefaultsAndChecksReportStyle(t *testing.T) {
	base := func() ResearchInput {
		return ResearchInput{Title: "WWII", Questions: []ResearchQuestion{{Label: "Causes", Question: "Why did it start?"}}, MaxSources: 40, MaxMinutes: 30}
	}
	in := base()
	if err := in.validate(); err != nil {
		t.Fatal(err)
	}
	if in.Length != "standard" || in.Tone != "report" {
		t.Fatalf("defaults %q %q", in.Length, in.Tone)
	}
	for _, bad := range []ResearchInput{{Length: "epic"}, {Tone: "limerick"}} {
		in := base()
		in.Length, in.Tone = bad.Length, bad.Tone
		if err := in.validate(); err == nil {
			t.Errorf("accepted %q %q", bad.Length, bad.Tone)
		}
	}
	for l := range reportLengths {
		for tone := range reportTones {
			in := base()
			in.Length, in.Tone = l, tone
			if err := in.validate(); err != nil {
				t.Errorf("%s %s: %v", l, tone, err)
			}
		}
	}
}

func TestResearchStyleNamesLengthAndTone(t *testing.T) {
	if s := researchStyle("in_depth", "news"); !strings.Contains(s, "6,000 to 9,000 words") || !strings.Contains(s, "news feature") {
		t.Fatalf("%q", s)
	}
	// A job from before the setting existed reads as the defaults.
	if s := researchStyle("", ""); !strings.Contains(s, "1,500 to 2,500 words") || !strings.Contains(s, "research report") {
		t.Fatalf("%q", s)
	}
}
