package artifacts_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
)

func TestCleanReviewPins(t *testing.T) {
	got, err := artifacts.CleanReviewPins(nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("nil pins = %v, %v; want an empty, non-nil slice", got, err)
	}
	got, err = artifacts.CleanReviewPins([]artifacts.ReviewPin{{X: 0, Y: 1, Text: "  Rename the button  "}})
	if err != nil || len(got) != 1 || got[0].Text != "Rename the button" {
		t.Fatalf("trimmed = %+v, %v", got, err)
	}

	tooMany := make([]artifacts.ReviewPin, artifacts.MaxReviewPins+1)
	for i := range tooMany {
		tooMany[i] = artifacts.ReviewPin{X: 0.5, Y: 0.5, Text: "x"}
	}
	for name, pins := range map[string][]artifacts.ReviewPin{
		"empty text":  {{X: 0.5, Y: 0.5, Text: "   "}},
		"too long":    {{X: 0.5, Y: 0.5, Text: strings.Repeat("é", artifacts.MaxReviewPinText+1)}},
		"x off right": {{X: 1.01, Y: 0.5, Text: "x"}},
		"y negative":  {{X: 0.5, Y: -0.1, Text: "x"}},
		"NaN":         {{X: math.NaN(), Y: 0.5, Text: "x"}},
		"too many":    tooMany,
	} {
		var v *apperr.ValidationError
		if _, err := artifacts.CleanReviewPins(pins); !errors.As(err, &v) || v.Field != "pins" {
			t.Errorf("%s: err = %v, want a pins validation error", name, err)
		}
	}
	if _, err := artifacts.CleanReviewPins([]artifacts.ReviewPin{{X: 0.5, Y: 0.5, Text: strings.Repeat("é", artifacts.MaxReviewPinText)}}); err != nil {
		t.Errorf("a pin at the length limit is valid: %v", err)
	}
}

func TestGatherReviewNote(t *testing.T) {
	if got := artifacts.GatherReviewNote("Looks good", nil); got != "Looks good" {
		t.Errorf("no pins = %q", got)
	}
	pins := []artifacts.ReviewPin{{X: 0.424, Y: 0.176, Text: "Rename to\n“New ticket”"}, {X: 1, Y: 0, Text: "Add a count"}}
	want := "Move the filters up\n\nPinned comments (% across, % down the drawing):\n1. (42%, 18%) Rename to “New ticket”\n2. (100%, 0%) Add a count"
	if got := artifacts.GatherReviewNote("Move the filters up", pins); got != want {
		t.Errorf("note =\n%s\nwant\n%s", got, want)
	}
	if got := artifacts.GatherReviewNote("", pins[1:]); got != "Pinned comments (% across, % down the drawing):\n1. (100%, 0%) Add a count" {
		t.Errorf("pins only = %q", got)
	}
}
