package wiki

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// ADR states parsed from a "Status: …" line.
const (
	ADRAccepted   = "accepted"
	ADRProposed   = "proposed"
	ADRSuperseded = "superseded"
	ADRDeprecated = "deprecated"
	ADRRejected   = "rejected"
	ADROther      = "other"
	ADRNone       = "none"
)

// ADRStatus is what an ADR's body says about its state.
type ADRStatus struct {
	Status        string   // "Accepted"; "" without a Status line
	State         string   // one of the ADR* states
	Date          string   // YYYY-MM-DD, or ""
	Ticket        string   // ticket key from "Ticket: [[KEY]]", or ""
	OpenQuestions []string // items of the "Open questions" section, as Markdown
}

var (
	statusLineRe = regexp.MustCompile(`(?im)^[ \t>]*[*_]*status[*_]*[ \t]*:[*_]*[ \t]*(.+)$`)
	dateRe       = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	ticketRe     = regexp.MustCompile(`(?i)ticket[*_]*\s*:[*_]*\s*\[\[([A-Za-z][A-Za-z0-9]*-\d+)`)
	withOpenQRe  = regexp.MustCompile(`(?i)[,;]?\s*(with|has)\s+open\s+questions?`)
	openQHeadRe  = regexp.MustCompile(`(?i)^#{1,6}\s+open\s+questions?\b`)
	listItemRe   = regexp.MustCompile(`^(\s*)(?:[-*+]|\d+[.)])\s+(.*)$`)
)

// ParseADR reads the Status line (status, date, ticket) and the Open
// questions section of an ADR body. Without parsed questions, a status that
// says "with open questions" keeps saying so.
func ParseADR(body string) ADRStatus {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	out := ADRStatus{State: ADRNone, OpenQuestions: openQuestions(body)}
	m := statusLineRe.FindStringSubmatch(body)
	if m == nil {
		return out
	}
	line := strings.TrimSpace(m[1])
	out.Date = dateRe.FindString(line)
	if t := ticketRe.FindStringSubmatch(line); t != nil {
		out.Ticket = strings.ToUpper(t[1])
	}
	// The status is the first clause: up to "(" or the end of the sentence.
	st := line
	if i := strings.IndexAny(st, "(\n"); i >= 0 {
		st = st[:i]
	}
	if i := strings.Index(st, ". "); i >= 0 {
		st = st[:i]
	}
	if len(out.OpenQuestions) > 0 {
		st = withOpenQRe.ReplaceAllString(st, "")
	}
	st = strings.Trim(strings.TrimSpace(st), ".,;:*_ ")
	out.Status = st
	first := strings.ToLower(strings.Trim(strings.SplitN(st+" ", " ", 2)[0], ".,;:"))
	switch first {
	case "accepted", "approved":
		out.State = ADRAccepted
	case "proposed", "draft":
		out.State = ADRProposed
	case "superseded":
		out.State = ADRSuperseded
	case "deprecated":
		out.State = ADRDeprecated
	case "rejected":
		out.State = ADRRejected
	default:
		out.State = ADROther
	}
	return out
}

// openQuestions lists the items of the first "Open questions" section; a
// line that isn't an item continues the previous one.
func openQuestions(body string) []string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, l := range lines {
		if openQHeadRe.MatchString(strings.TrimSpace(l)) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	var out []string
	indent := -1
	fence := false
	for _, l := range lines[start:] {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if headingRe.MatchString(t) {
			break
		}
		if t == "" {
			continue
		}
		if m := listItemRe.FindStringSubmatch(l); m != nil && (indent < 0 || len(m[1]) <= indent) {
			indent = len(m[1])
			out = append(out, strings.TrimSpace(m[2]))
		} else if len(out) > 0 {
			out[len(out)-1] += " " + t
		}
	}
	return out
}

// ADR is one architecture decision as listed in the ADR index.
type ADR struct {
	ID        uuid.UUID
	Title     string
	Path      string // wiki pages
	DisplayID string // ADR documents
	Href      string
	UpdatedAt time.Time
	ADRStatus
}

// ListADRs lists ADR documents and wiki pages titled "ADR …": those with open
// questions first, then the most recently updated.
func ListADRs(ctx context.Context, q *store.Queries, ws uuid.UUID) ([]ADR, error) {
	rows, err := q.ListADRDocs(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := make([]ADR, 0, len(rows))
	for _, r := range rows {
		a := ADR{ID: r.ID, Title: r.Title, UpdatedAt: r.UpdatedAt, ADRStatus: ParseADR(deref(r.BodyMd))}
		if r.Type == "note" {
			a.Path = deref(r.Path)
			a.Href = links.NoteHref(a.Path)
		} else {
			a.DisplayID = deref(artifacts.DisplayID(r.Type, r.Number))
			a.Href = "/artifacts/" + r.ID.String()
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].OpenQuestions) > 0 && len(out[j].OpenQuestions) == 0
	})
	return out, nil
}
