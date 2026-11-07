package wiki

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Patterns are curated, workspace-wide, atomic notes (architecture patterns,
// coding styles): Patterns/<Topic>/<Title>. Agents propose them under
// Patterns/Proposed/<Title>; an agent with wiki:write allowed publishes or
// rejects them. They are separate from agent run notes (agents/<handle>/) and
// project decisions (ADRs).
const (
	PatternsRoot  = "Patterns/"
	ProposedRoot  = PatternsRoot + "Proposed/"
	maxTitleRunes = 80
)

// PatternTopics are the folders under Patterns/.
var PatternTopics = []string{"Go", "React", "Postgres", "Testing", "API", "Architecture", "Process"}

// PatternTemplate is the body layout proposals follow after the header.
const PatternTemplate = "## When to use\n\n## Rule\n\n## Example\n\n## When not to use\n\n## Related\n"

// ValidTopic returns the canonical spelling of a topic.
func ValidTopic(t string) (string, error) {
	t = strings.TrimSpace(t)
	for _, k := range PatternTopics {
		if strings.EqualFold(k, t) {
			return k, nil
		}
	}
	return "", apperr.Invalid("topic", "must be one of %s", strings.Join(PatternTopics, ", "))
}

// ValidPatternTitle checks a short, linkable title that can be one path segment.
func ValidPatternTitle(t string) (string, error) {
	t = strings.Join(strings.Fields(t), " ")
	switch {
	case t == "":
		return "", apperr.Invalid("title", "is required, e.g. Wrap errors with context")
	case utf8.RuneCountInString(t) > maxTitleRunes:
		return "", apperr.Invalid("title", "must be at most %d characters: keep it short and imperative", maxTitleRunes)
	case strings.ContainsAny(t, "/\\[]#|^"):
		return "", apperr.Invalid("title", "can't contain / \\ [ ] # | or ^")
	case strings.EqualFold(t, "proposed") || strings.HasPrefix(t, "."):
		return "", apperr.Invalid("title", "isn't a valid pattern title")
	}
	return t, nil
}

// PatternPath is the path of a published pattern.
func PatternPath(topic, title string) (string, error) {
	tp, err := ValidTopic(topic)
	if err != nil {
		return "", err
	}
	ti, err := ValidPatternTitle(title)
	if err != nil {
		return "", err
	}
	return PatternsRoot + tp + "/" + ti, nil
}

// ProposedPath is the path of a pattern proposal.
func ProposedPath(title string) (string, error) {
	ti, err := ValidPatternTitle(title)
	if err != nil {
		return "", err
	}
	return ProposedRoot + ti, nil
}

// ParsePatternPath splits a path into topic and title. proposed is true for
// Patterns/Proposed/<Title> (topic is then ""); ok is false for anything else.
func ParsePatternPath(path string) (topic, title string, proposed, ok bool) {
	segs := strings.Split(links.NormalizePath(path), "/")
	if len(segs) != 3 || !strings.EqualFold(segs[0], "Patterns") {
		return "", "", false, false
	}
	if strings.EqualFold(segs[1], "Proposed") {
		return "", segs[2], true, true
	}
	t, err := ValidTopic(segs[1])
	if err != nil {
		return "", "", false, false
	}
	return t, segs[2], false, true
}

// IsPatternPath reports whether a path lives in the Patterns/ area.
func IsPatternPath(path string) bool {
	return strings.HasPrefix(strings.ToLower(links.NormalizePath(path)), "patterns/")
}

// NormalizeTags lowercases, dedupes and checks the 2-5 tags of a pattern.
func NormalizeTags(tags []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.ToLower(strings.Join(strings.Fields(strings.Trim(t, "#, ")), "-"))
		if t == "" || seen[t] {
			continue
		}
		if strings.ContainsAny(t, ",[]|") || utf8.RuneCountInString(t) > 30 {
			return nil, apperr.Invalid("tags", "%q isn't a valid tag: use short lowercase words", t)
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) < 2 || len(out) > 5 {
		return nil, apperr.Invalid("tags", "give 2 to 5 lowercase tags, e.g. errors, context")
	}
	return out, nil
}

// PatternHeader is the Topic/Tags header every pattern body starts with.
func PatternHeader(topic string, tags []string) string {
	return "Topic: " + topic + "\nTags: " + strings.Join(tags, ", ") + "\n\n"
}

// ParseHeader reads the Topic:/Tags: lines at the top of a body. rest is the
// body after them (and the blank lines that follow).
func ParseHeader(body string) (topic string, tags []string, rest string) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	i := 0
	for ; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if l == "" && topic == "" && tags == nil {
			continue
		}
		if v, ok := cutField(l, "topic"); ok {
			topic = v
		} else if v, ok := cutField(l, "tags"); ok {
			for t := range strings.SplitSeq(v, ",") {
				if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
					tags = append(tags, t)
				}
			}
		} else {
			break
		}
	}
	return topic, tags, strings.TrimLeft(strings.Join(lines[i:], "\n"), "\n")
}

func cutField(line, name string) (string, bool) {
	if len(line) > len(name) && strings.EqualFold(line[:len(name)], name) && line[len(name)] == ':' {
		return strings.TrimSpace(line[len(name)+1:]), true
	}
	return "", false
}

var (
	headingRe = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	relatedRe = regexp.MustCompile(`(?im)^#{1,6}\s+related\s*$`)
)

// Summary is the first line of "When to use" (else of the body after the header).
func Summary(body string) string {
	_, _, rest := ParseHeader(body)
	lines := strings.Split(rest, "\n")
	pick := func(from int) string {
		for _, l := range lines[from:] {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if headingRe.MatchString(l) {
				break
			}
			l = strings.TrimSpace(strings.TrimLeft(l, "-*>"))
			if r := []rune(l); len(r) > 200 {
				l = string(r[:200]) + "…"
			}
			return l
		}
		return ""
	}
	for i, l := range lines {
		if m := headingRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil && strings.EqualFold(m[1], "when to use") {
			if s := pick(i + 1); s != "" {
				return s
			}
		}
	}
	return pick(0)
}

// ComposeProposal builds a proposal body: header, the agent's text and, when
// the pattern came from a ticket, a [[KEY]] link in Related.
func ComposeProposal(topic string, tags []string, body, sourceTicket string) string {
	_, _, rest := ParseHeader(body)
	rest = strings.TrimSpace(rest)
	if sourceTicket != "" {
		line := "- Source: [[" + sourceTicket + "]]"
		if loc := relatedRe.FindStringIndex(rest); loc != nil {
			rest = rest[:loc[1]] + "\n" + line + rest[loc[1]:]
		} else {
			rest += "\n\n## Related\n\n" + line
		}
	}
	return PatternHeader(topic, tags) + rest + "\n"
}

// ------------------------------------------------------------ search

// Pattern is one published pattern as listed or found.
type Pattern struct {
	ID      uuid.UUID `json:"-"`
	Title   string    `json:"title"`
	Path    string    `json:"path"`
	Topic   string    `json:"topic"`
	Tags    []string  `json:"tags"`
	Summary string    `json:"summary"`
	Snippet string    `json:"snippet"`
	Score   float64   `json:"score"`
}

// FindPatterns searches published patterns (never proposals or other notes)
// with typo-tolerant matching on title, tags and path plus full text, boosted
// by links. An empty text lists them by topic then title. topic filters to one
// folder; limit defaults to 10, max 50.
func FindPatterns(ctx context.Context, q *store.Queries, ws uuid.UUID, text, topic string, limit int) ([]Pattern, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return findPatterns(ctx, q, ws, text, topic, limit)
}

func findPatterns(ctx context.Context, q *store.Queries, ws uuid.UUID, text, topic string, limit int) ([]Pattern, error) {
	var tp *string
	if strings.TrimSpace(topic) != "" {
		t, err := ValidTopic(topic)
		if err != nil {
			return nil, err
		}
		tp = &t
	}
	rows, err := q.FindPatterns(ctx, store.FindPatternsParams{Q: strings.TrimSpace(text), WorkspaceID: ws, Topic: tp, Lim: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Pattern, 0, len(rows))
	for _, r := range rows {
		body := deref(r.BodyMd)
		path := deref(r.Path)
		topic, _, _, _ := ParsePatternPath(path)
		_, tags, _ := ParseHeader(body)
		if tags == nil {
			tags = []string{}
		}
		out = append(out, Pattern{ID: r.ID, Title: r.Title, Path: path, Topic: topic, Tags: tags, Summary: Summary(body), Snippet: r.Snippet, Score: r.Score})
	}
	return out, nil
}

// TagCount is one entry of a tag facet.
type TagCount struct {
	Tag   string
	Count int
}

// maxPatternScan bounds how many matches BrowsePatterns reads to build the
// facet before filtering by tag.
const maxPatternScan = 1000

// BrowsePatterns is FindPatterns with a tag filter (every tag must match) and
// the tag facet of the text and topic matches, before the tag filter, most
// used first (ties by name).
func BrowsePatterns(ctx context.Context, q *store.Queries, ws uuid.UUID, text, topic string, tags []string, limit int) ([]Pattern, []TagCount, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	all, err := findPatterns(ctx, q, ws, text, topic, maxPatternScan)
	if err != nil {
		return nil, nil, err
	}
	want := map[string]bool{}
	for _, t := range tags {
		if t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))); t != "" {
			want[t] = true
		}
	}
	counts := map[string]int{}
	out := []Pattern{}
	for _, p := range all {
		has := 0
		seen := map[string]bool{}
		for _, t := range p.Tags {
			if seen[t] {
				continue
			}
			seen[t] = true
			counts[t]++
			if want[t] {
				has++
			}
		}
		if has == len(want) && len(out) < limit {
			out = append(out, p)
		}
	}
	facet := make([]TagCount, 0, len(counts))
	for t, n := range counts {
		facet = append(facet, TagCount{Tag: t, Count: n})
	}
	sort.Slice(facet, func(i, j int) bool {
		if facet[i].Count != facet[j].Count {
			return facet[i].Count > facet[j].Count
		}
		return facet[i].Tag < facet[j].Tag
	})
	return out, facet, nil
}

// ------------------------------------------------------------ proposals

// CanCurate reports whether an agent may publish and reject patterns: its
// wiki:write scope must be allowed.
func CanCurate(scopes map[string]agentdef.Mode) bool {
	mode, set := agentdef.ModeFor(scopes, "wiki:write")
	return set && mode == agentdef.Allowed
}

func errNotCurator() error {
	return apperr.Invalid("scope", "only agents whose wiki:write scope is allowed can publish or reject patterns; propose with propose_pattern instead")
}

// publishedByTitle finds a published pattern called title, in any topic.
func publishedByTitle(ctx context.Context, q *store.Queries, ws uuid.UUID, title string) (*store.Artifact, error) {
	notes, err := q.FindNotesByTitle(ctx, store.FindNotesByTitleParams{WorkspaceID: ws, Title: title})
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		if n.Path != nil {
			if _, _, proposed, ok := ParsePatternPath(*n.Path); ok && !proposed {
				return &n, nil
			}
		}
	}
	return nil, nil
}

// Proposal is an agent's suggested pattern.
type Proposal struct {
	Title        string
	Topic        string
	Tags         []string
	Body         string
	SourceTicket string // key, linked in Related
}

// ProposePattern writes Patterns/Proposed/<Title>. It is always writable (like
// an agent's memory), whatever wiki:write says. The same agent can re-propose
// to revise; another agent's proposal or a published pattern with that title is
// an error.
func (s *Service) ProposePattern(ctx context.Context, agent store.Agent, in Proposal) (Result, error) {
	topic, err := ValidTopic(in.Topic)
	if err != nil {
		return Result{}, err
	}
	title, err := ValidPatternTitle(in.Title)
	if err != nil {
		return Result{}, err
	}
	tags, err := NormalizeTags(in.Tags)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(in.Body) == "" {
		return Result{}, apperr.Invalid("body", "is required: follow the template (When to use, Rule, Example, When not to use, Related)")
	}
	path := ProposedRoot + title
	body := ComposeProposal(topic, tags, in.Body, in.SourceTicket)
	var res Result
	err = s.tx(ctx, func(q *store.Queries) error {
		ws := agent.WorkspaceID
		if pub, err := publishedByTitle(ctx, q, ws, title); err != nil {
			return err
		} else if pub != nil {
			return apperr.Invalid("title", "a pattern called %q is already published at %s; improve it with write_note (mode replace) or propose a different title", title, deref(pub.Path))
		}
		mode := ModeCreate
		if ex, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: path}); err == nil && ex.CurrentVersionID != nil {
			if ex.AuthorType != "agent" || ex.AuthorID != agent.ID {
				return apperr.Invalid("title", "%q has already been proposed by someone else; pick a different title or wait for it to be reviewed", title)
			}
			mode = ModeReplace
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		res, err = s.write(ctx, q, ws, artifacts.Actor{Type: "agent", ID: agent.ID}, Save{Path: path, Body: body, Mode: mode, Note: "Proposed pattern"}, artifacts.Published)
		return err
	})
	return res, err
}

// ProposedPattern is one waiting proposal.
type ProposedPattern struct {
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	Proposer  string    `json:"proposer"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListProposed lists proposals waiting for review, oldest first.
func (s *Service) ListProposed(ctx context.Context, ws uuid.UUID) ([]ProposedPattern, error) {
	q := store.New(s.Pool)
	notes, err := q.ListNotes(ctx, ws)
	if err != nil {
		return nil, err
	}
	var rows []store.ListNotesRow
	var agentIDs []uuid.UUID
	for _, n := range notes {
		if n.Published && n.Path != nil && strings.HasPrefix(strings.ToLower(*n.Path), strings.ToLower(ProposedRoot)) {
			rows = append(rows, n)
			if n.AuthorType == "agent" {
				agentIDs = append(agentIDs, n.AuthorID)
			}
		}
	}
	handles := map[uuid.UUID]string{}
	if len(agentIDs) > 0 {
		as, err := q.AgentsByIDs(ctx, agentIDs)
		if err != nil {
			return nil, err
		}
		for _, a := range as {
			handles[a.ID] = "@" + a.Handle
		}
	}
	out := make([]ProposedPattern, 0, len(rows))
	for _, n := range rows {
		who := handles[n.AuthorID]
		if who == "" {
			who = "a person"
		}
		out = append(out, ProposedPattern{Title: n.Title, Path: *n.Path, Proposer: who, UpdatedAt: n.UpdatedAt})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].UpdatedAt.Before(out[j-1].UpdatedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// PublishInput says which proposal to publish and what to change on the way.
type PublishInput struct {
	ProposedTitle string
	Title         string // optional: rename
	Topic         string // optional: override the proposal's topic
	Body          string // optional: replaces the proposal's body
}

// loadProposal locks the proposal page and returns it with its current body.
func loadProposal(ctx context.Context, q *store.Queries, ws uuid.UUID, title string) (store.Artifact, string, error) {
	path, err := ProposedPath(title)
	if err != nil {
		return store.Artifact{}, "", err
	}
	a, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: path})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.CurrentVersionID == nil) {
		return store.Artifact{}, "", apperr.Invalid("proposed_title", "no proposed pattern called %q; list_proposed_patterns shows what is waiting", title)
	} else if err != nil {
		return store.Artifact{}, "", err
	}
	if a, err = q.LockArtifact(ctx, store.LockArtifactParams{ID: a.ID, WorkspaceID: ws}); err != nil {
		return store.Artifact{}, "", err
	}
	body, _, err := current(ctx, q, a)
	return a, body, err
}

// removeProposal deletes a proposal for good: the page, its versions and
// the links it made. (The artifacts service has no archive state for notes;
// deleting also keeps it out of lists and search.)
func removeProposal(ctx context.Context, q *store.Queries, a store.Artifact) error {
	if err := q.DeleteLinksFromSource(ctx, store.DeleteLinksFromSourceParams{SourceKind: "artifact", SourceID: a.ID}); err != nil {
		return err
	}
	return q.DeleteArtifact(ctx, store.DeleteArtifactParams{ID: a.ID, WorkspaceID: a.WorkspaceID})
}

// PublishPattern moves a proposal to Patterns/<Topic>/<Title> (optionally with
// an edited title, topic or body) and deletes the proposal. It fails if a
// published page already has that path or title.
func (s *Service) PublishPattern(ctx context.Context, agent store.Agent, scopes map[string]agentdef.Mode, in PublishInput) (Result, error) {
	if !CanCurate(scopes) {
		return Result{}, errNotCurator()
	}
	var res Result
	err := s.tx(ctx, func(q *store.Queries) error {
		ws := agent.WorkspaceID
		prop, cur, err := loadProposal(ctx, q, ws, in.ProposedTitle)
		if err != nil {
			return err
		}
		body := cur
		if strings.TrimSpace(in.Body) != "" {
			body = in.Body
		}
		hTopic, hTags, rest := ParseHeader(body)
		// An edited body may omit the header: fall back to the proposal's.
		pTopic, pTags, _ := ParseHeader(cur)
		if len(hTags) == 0 {
			hTags = pTags
		}
		hTopic = firstNonEmpty(hTopic, pTopic)
		topic := firstNonEmpty(in.Topic, hTopic)
		if topic == "" {
			return apperr.Invalid("topic", "the proposal has no Topic line: pass topic (%s)", strings.Join(PatternTopics, ", "))
		}
		title := firstNonEmpty(in.Title, prop.Title)
		path, err := PatternPath(topic, title)
		if err != nil {
			return err
		}
		tags, err := NormalizeTags(hTags)
		if err != nil {
			return err
		}
		topic, title = pathParts(path)
		if pub, err := publishedByTitle(ctx, q, ws, title); err != nil {
			return err
		} else if pub != nil {
			return apperr.Conflict("a pattern called %q is already published at %s; merge into it with write_note (mode replace) and reject_pattern this proposal", title, deref(pub.Path))
		}
		if ex, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: path}); err == nil && ex.ID != prop.ID {
			return apperr.Conflict("%s already exists; merge into it with write_note (mode replace) and reject_pattern this proposal", path)
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		res, err = s.write(ctx, q, ws, artifacts.Actor{Type: "agent", ID: agent.ID},
			Save{Path: path, Body: PatternHeader(topic, tags) + strings.TrimSpace(rest) + "\n", Mode: ModeCreate, Note: "Published from " + ProposedRoot + prop.Title}, artifacts.Published)
		if err != nil {
			return err
		}
		return removeProposal(ctx, q, prop)
	})
	return res, err
}

// RejectPattern deletes a proposal. The caller relays the reason.
func (s *Service) RejectPattern(ctx context.Context, agent store.Agent, scopes map[string]agentdef.Mode, title string) (proposer string, err error) {
	if !CanCurate(scopes) {
		return "", errNotCurator()
	}
	err = s.tx(ctx, func(q *store.Queries) error {
		prop, _, err := loadProposal(ctx, q, agent.WorkspaceID, title)
		if err != nil {
			return err
		}
		proposer = "a person"
		if prop.AuthorType == "agent" {
			if as, err := q.AgentsByIDs(ctx, []uuid.UUID{prop.AuthorID}); err == nil && len(as) == 1 {
				proposer = "@" + as[0].Handle
			}
		}
		return removeProposal(ctx, q, prop)
	})
	return proposer, err
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

func pathParts(path string) (topic, title string) {
	t, ti, _, _ := ParsePatternPath(path)
	return t, ti
}
