package chatmodels

import (
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// A harness fixes its effort at launch, so a chat without a chosen effort gets
// one per turn from the message itself, before the model sees it.

var (
	deepAsk = regexp.MustCompile(`\b(thorough(ly)?|deep[- ]dive|in[- ]depth|comprehensive(ly)?|architecture|redesign|refactor|migrate|migration|audit|root cause|think (hard|carefully))\b`)
	workAsk = regexp.MustCompile(`\b(implement|fix|debug|investigate|compare|analy[sz]e|research|failing|broken|bugs?|crash(es|ing)?)\b`)
	// asked finds a verb used as a request. Words like "make" or "test" are
	// only work when asked for: "does that make sense" and "the test passed"
	// aren't.
	asked   = regexp.MustCompile(`(^|[.!?,;:\n]\s*|\b(and|then|also|please|pls|you|let'?s|lets|need to|needs to|want to|like to|have to|should|must|go ahead and)\s+)(implement|fix|debug|investigate|compare|analy[sz]e|research|add|build|change|update|write|create|review|set|make|remove|rename|test|plan|design|refactor|redesign|migrate|audit)\b`)
	thanks  = regexp.MustCompile(`^(thanks|thank you|thx|ty|nice|great|awesome|perfect|cool|sweet|love it|looks good|good (job|work))\b`)
	onward  = regexp.MustCompile(`\b(but|now|next|let'?s)\b`)
	codeish = regexp.MustCompile("```|\\b[\\w-]+\\.(go|ts|tsx|js|py|sql|md|ya?ml|json)\\b")
)

// AutoLevel is the effort a message calls for: low for quick conversation and
// thanks, scaling up with the size of the ask.
func AutoLevel(text string) string {
	t := strings.ToLower(text)
	words := len(strings.Fields(t))
	if words <= 12 && thanks.MatchString(t) && !strings.Contains(t, "?") && !onward.MatchString(t) && !asked.MatchString(t) && !codeish.MatchString(t) {
		return "low"
	}
	switch {
	case deepAsk.MatchString(t) || words > 200:
		return "xhigh"
	case workAsk.MatchString(t) || asked.MatchString(t) || codeish.MatchString(t) || words > 60:
		return "high"
	case words > 12 || (words >= 6 && strings.Contains(t, "?")):
		return "medium"
	}
	return "low"
}

// MentionsCode reports whether lowered text quotes code or names a source file.
func MentionsCode(text string) bool { return codeish.MatchString(text) }

var offered = struct {
	sync.Mutex
	byHarness map[string][]Model
}{byHarness: map[string][]Model{}}

func offeredKey(ws uuid.UUID, harness string) string { return ws.String() + "/" + harness }

// rememberOffered keeps each harness's latest listing: discovery can start a
// CLI, too slow to repeat for every message.
func rememberOffered(ws uuid.UUID, list []Harness) {
	offered.Lock()
	defer offered.Unlock()
	for _, h := range list {
		if len(h.Models) > 0 {
			offered.byHarness[offeredKey(ws, h.Harness)] = h.Models
		}
	}
}

// AutoEffort is the effort to run at for level, limited to what the model
// offers: the highest one at or below level, else the lowest it has. It is
// empty (the harness default) when the model has no effort control or the
// catalog hasn't been listed yet. A model the catalog doesn't list (the
// definition's alias) gets its harness default model's efforts.
func AutoEffort(ws uuid.UUID, harness, model, level string) string {
	offered.Lock()
	models := offered.byHarness[offeredKey(ws, harness)]
	offered.Unlock()
	var efforts []string
	for _, m := range models {
		if m.ID == model {
			efforts = m.Efforts
			break
		}
		if m.Default {
			efforts = m.Efforts
		}
	}
	if len(efforts) == 0 {
		return ""
	}
	pick := efforts[0]
	for _, e := range efforts {
		if slices.Index(agentdef.Efforts, e) <= slices.Index(agentdef.Efforts, level) {
			pick = e
		}
	}
	return pick
}
