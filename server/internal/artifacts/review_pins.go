package artifacts

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// ReviewPin is a comment a reviewer pinned on a mockup version. X and Y are
// fractions of the drawing's width and height, so a pin stays put at any zoom.
type ReviewPin struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Text string  `json:"text"`
}

// Limits on pinned review comments.
const (
	MaxReviewPins    = 50
	MaxReviewPinText = 500
)

// CleanReviewPins trims each pin's text and rejects too many pins, empty or
// overlong comments, and positions off the drawing. It never returns nil.
func CleanReviewPins(pins []ReviewPin) ([]ReviewPin, error) {
	if len(pins) > MaxReviewPins {
		return nil, apperr.Invalid("pins", "at most %d pinned comments", MaxReviewPins)
	}
	out := make([]ReviewPin, 0, len(pins))
	for i, p := range pins {
		p.Text = strings.TrimSpace(p.Text)
		switch {
		case p.Text == "":
			return nil, apperr.Invalid("pins", "pin %d has no comment", i+1)
		case utf8.RuneCountInString(p.Text) > MaxReviewPinText:
			return nil, apperr.Invalid("pins", "pin %d is longer than %d characters", i+1, MaxReviewPinText)
		case !(p.X >= 0 && p.X <= 1 && p.Y >= 0 && p.Y <= 1): // also rejects NaN
			return nil, apperr.Invalid("pins", "pin %d is off the drawing", i+1)
		}
		out = append(out, p)
	}
	return out, nil
}

// GatherReviewNote appends the pins to the reviewer's note as a numbered list,
// each with its position in percent, so anything that reads review_note (the
// waiting agent, the ticket's mockup gate) sees them.
func GatherReviewNote(note string, pins []ReviewPin) string {
	if len(pins) == 0 {
		return note
	}
	var b strings.Builder
	if note != "" {
		b.WriteString(note)
		b.WriteString("\n\n")
	}
	b.WriteString("Pinned comments (% across, % down the drawing):")
	for i, p := range pins {
		fmt.Fprintf(&b, "\n%d. (%d%%, %d%%) %s", i+1, int(math.Round(p.X*100)), int(math.Round(p.Y*100)), strings.Join(strings.Fields(p.Text), " "))
	}
	return b.String()
}
