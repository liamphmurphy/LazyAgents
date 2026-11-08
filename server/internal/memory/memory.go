// Package memory is the personal assistant's per-person memory store, kept
// apart from the shared wiki. Every operation takes an Owner and touches only
// that person's entries: there is no cross-person read or write.
package memory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Types are the kinds of memory an entry can be.
var Types = []string{"preference", "project", "person", "life", "work", "fact", "other"}

const (
	maxTitle   = 200
	maxSummary = 300
	maxBody    = 20000
	maxTags    = 12
	maxTag     = 40
	maxReason  = 300

	// IndexSize caps the always-loaded index: enough to recognise what's
	// remembered, small enough to load every turn.
	IndexSize = 50
	// Near-duplicate thresholds (pg_trgm similarity, 0–1).
	dupTitle   = 0.6
	dupSummary = 0.75
)

// Owner is the one person whose memory an operation reads or writes.
type Owner struct {
	WorkspaceID uuid.UUID
	UserID      uuid.UUID
}

// Source records where a memory came from.
type Source struct {
	Label  string // e.g. "chat 01a1…" or "manual"
	ChatID *uuid.UUID
	RunID  *uuid.UUID
}

// Input is a memory to save. Type defaults to "other".
type Input struct {
	Title   string
	Summary string
	Body    string
	Tags    []string
	Type    string
}

// Patch changes some fields of an entry; nil leaves a field alone.
type Patch struct {
	Title   *string
	Summary *string
	Body    *string
	Tags    *[]string
	Type    *string
}

type Service struct{ Q *store.Queries }

func New(q *store.Queries) *Service { return &Service{Q: q} }

// SaveResult says whether Save added an entry or updated a near-duplicate;
// Previous is that entry as it was before the update.
type SaveResult struct {
	Memory   store.Memory
	Updated  bool
	Previous *store.Memory
}

// Save adds a memory, or updates the person's near-duplicate entry instead.
// Secrets are refused.
func (s *Service) Save(ctx context.Context, o Owner, in Input, src Source) (SaveResult, error) {
	typeGiven := strings.TrimSpace(in.Type) != ""
	in, err := normalizeInput(in)
	if err != nil {
		return SaveResult{}, err
	}
	dup, err := s.Q.NearDuplicateMemory(ctx, store.NearDuplicateMemoryParams{
		WorkspaceID: o.WorkspaceID, UserID: o.UserID, Title: in.Title, Summary: in.Summary,
		TitleThreshold: dupTitle, SummaryThreshold: dupSummary,
	})
	switch {
	case err == nil:
		prev := dup.Memory
		p := Patch{Tags: new(mergeTags(prev.Tags, in.Tags))}
		// A match on the summary alone keeps the entry's title.
		if dup.TitleScore >= dupTitle {
			p.Title = &in.Title
		}
		if in.Summary != "" {
			p.Summary = &in.Summary
		}
		if in.Body != "" {
			p.Body = &in.Body
		}
		if typeGiven {
			p.Type = &in.Type
		}
		u, err := s.update(ctx, o, prev.ID, p, src)
		return SaveResult{Memory: u, Updated: true, Previous: &prev}, err
	case !errors.Is(err, pgx.ErrNoRows):
		return SaveResult{}, err
	}
	m, err := s.Q.CreateMemory(ctx, store.CreateMemoryParams{
		WorkspaceID: o.WorkspaceID, UserID: o.UserID, Title: in.Title, Summary: in.Summary, Body: in.Body,
		Tags: in.Tags, Type: in.Type, Source: src.Label, SourceChatID: src.ChatID, SourceRunID: src.RunID,
	})
	return SaveResult{Memory: m}, err
}

// Update edits an entry. A non-empty src.Label replaces the entry's source.
func (s *Service) Update(ctx context.Context, o Owner, id uuid.UUID, p Patch, src Source) (store.Memory, error) {
	if p.Title != nil {
		p.Title = new(strings.TrimSpace(*p.Title))
		if err := checkLen("title", *p.Title, 1, maxTitle); err != nil {
			return store.Memory{}, err
		}
	}
	if p.Summary != nil {
		p.Summary = new(oneLine(*p.Summary))
		if err := checkLen("summary", *p.Summary, 0, maxSummary); err != nil {
			return store.Memory{}, err
		}
	}
	if p.Body != nil {
		p.Body = new(strings.TrimSpace(*p.Body))
		if err := checkLen("body", *p.Body, 0, maxBody); err != nil {
			return store.Memory{}, err
		}
	}
	if p.Tags != nil {
		tags, err := normalizeTags(*p.Tags)
		if err != nil {
			return store.Memory{}, err
		}
		p.Tags = &tags
	}
	if p.Type != nil && !slices.Contains(Types, *p.Type) {
		return store.Memory{}, apperr.Invalid("type", "must be one of %s", strings.Join(Types, ", "))
	}
	if err := checkSecrets(deref(p.Title), deref(p.Summary), deref(p.Body), derefTags(p.Tags)); err != nil {
		return store.Memory{}, err
	}
	return s.update(ctx, o, id, p, src)
}

func (s *Service) update(ctx context.Context, o Owner, id uuid.UUID, p Patch, src Source) (store.Memory, error) {
	arg := store.UpdateMemoryParams{ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID,
		Title: p.Title, Summary: p.Summary, Body: p.Body, Type: p.Type}
	if p.Tags != nil {
		arg.Tags = *p.Tags
		if arg.Tags == nil {
			arg.Tags = []string{}
		}
	}
	if src.Label != "" {
		arg.Source, arg.SourceChatID, arg.SourceRunID = &src.Label, src.ChatID, src.RunID
	}
	m, err := s.Q.UpdateMemory(ctx, arg)
	return m, notFound(err)
}

// Supersede retires an entry in favour of another live entry of the same
// person, recording why, or restores it when by is nil. A restored entry is
// never merged by consolidation again.
func (s *Service) Supersede(ctx context.Context, o Owner, id uuid.UUID, by *uuid.UUID, reason string) (store.Memory, error) {
	if by != nil && *by == id {
		return store.Memory{}, apperr.Invalid("superseded_by", "an entry can't supersede itself")
	}
	m, err := s.Q.SetMemorySupersededBy(ctx, store.SetMemorySupersededByParams{ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID, SupersededBy: by,
		// Only a person restores (the memory manager), and that exempts the
		// entry from consolidation.
		RetiredReason: truncate(oneLine(reason), maxReason), RestoredByPerson: by == nil})
	if errors.Is(err, pgx.ErrNoRows) && by != nil {
		if _, gerr := s.Get(ctx, o, id); gerr != nil {
			return m, gerr
		}
		return m, apperr.Invalid("superseded_by", "must be another of your current memories")
	}
	return m, notFound(err)
}

func (s *Service) Get(ctx context.Context, o Owner, id uuid.UUID) (store.Memory, error) {
	m, err := s.Q.GetMemory(ctx, store.GetMemoryParams{ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID})
	return m, notFound(err)
}

// Query filters List and Search. An empty Text lists newest first.
type Query struct {
	Text           string
	Type           string
	IncludeRetired bool
	Limit, Offset  int
}

// Hit is a memory with its search rank (0 when listing).
type Hit struct {
	store.Memory
	Rank float64
}

// Find searches (ranked) when q.Text is set, and lists newest first otherwise.
func (s *Service) Find(ctx context.Context, o Owner, q Query) ([]Hit, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 20
	}
	var typ *string
	if q.Type != "" {
		if !slices.Contains(Types, q.Type) {
			return nil, apperr.Invalid("type", "must be one of %s", strings.Join(Types, ", "))
		}
		typ = &q.Type
	}
	text := strings.TrimSpace(q.Text)
	out := []Hit{}
	if text == "" {
		rows, err := s.Q.ListMemories(ctx, store.ListMemoriesParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID,
			IncludeRetired: q.IncludeRetired, Type: typ, Lim: int32(q.Limit), Off: int32(max(q.Offset, 0))})
		for _, m := range rows {
			out = append(out, Hit{Memory: m})
		}
		return out, err
	}
	rows, err := s.Q.SearchMemories(ctx, store.SearchMemoriesParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID,
		Q: text, IncludeRetired: q.IncludeRetired, Type: typ, Lim: int32(q.Limit)})
	for _, r := range rows {
		out = append(out, Hit{Memory: r.Memory, Rank: r.Rank})
	}
	return out, err
}

// Count is how many entries match q's type and retired filter (not its text).
func (s *Service) Count(ctx context.Context, o Owner, q Query) (int, error) {
	var typ *string
	if q.Type != "" {
		typ = &q.Type
	}
	n, err := s.Q.CountMemories(ctx, store.CountMemoriesParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID, IncludeRetired: q.IncludeRetired, Type: typ})
	return int(n), err
}

// IndexEntry is one line of the always-loaded index.
type IndexEntry struct {
	ID      uuid.UUID `json:"id"`
	Title   string    `json:"title"`
	Summary string    `json:"summary"`
	Type    string    `json:"type"`
}

// Index lists the person's most recently changed live entries, titles and
// summaries only, and the total number of live entries.
func (s *Service) Index(ctx context.Context, o Owner) ([]IndexEntry, int, error) {
	hits, err := s.Find(ctx, o, Query{Limit: IndexSize})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.Count(ctx, o, Query{})
	if err != nil {
		return nil, 0, err
	}
	out := make([]IndexEntry, len(hits))
	for i, h := range hits {
		out[i] = IndexEntry{ID: h.ID, Title: h.Title, Summary: h.Summary, Type: h.Type}
	}
	return out, total, nil
}

// IndexMarkdown renders the index for an assistant's context.
func IndexMarkdown(entries []IndexEntry, total int) string {
	var b strings.Builder
	b.WriteString("### Memory index\n\n")
	if total == 0 {
		b.WriteString("Nothing remembered yet.\n")
		return b.String()
	}
	b.WriteString("Titles and summaries only. Use read_memory(id) for the full entry, search_memory to find more.\n\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "- `%s` [%s] %s", e.ID, e.Type, e.Title)
		if e.Summary != "" {
			fmt.Fprintf(&b, ": %s", e.Summary)
		}
		b.WriteString("\n")
	}
	if total > len(entries) {
		b.WriteString("\n…and more not listed here; search_memory finds them.\n")
	}
	return b.String()
}

// Forget deletes an entry and everything it superseded.
func (s *Service) Forget(ctx context.Context, o Owner, id uuid.UUID) error {
	n, err := s.Q.DeleteMemory(ctx, store.DeleteMemoryParams{ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID})
	if err == nil && n == 0 {
		return apperr.ErrNotFound
	}
	return err
}

// DeleteAll forgets everything the person has and returns how many entries
// went (superseded entries cascade, so only top-level rows are counted).
func (s *Service) DeleteAll(ctx context.Context, o Owner) (int, error) {
	n, err := s.Q.DeleteAllMemories(ctx, store.DeleteAllMemoriesParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID})
	return int(n), err
}

// Export returns every entry the person has, retired ones included.
func (s *Service) Export(ctx context.Context, o Owner) ([]store.Memory, error) {
	return s.Q.ExportMemories(ctx, store.ExportMemoriesParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID})
}

func normalizeInput(in Input) (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Summary = oneLine(in.Summary)
	in.Body = strings.TrimSpace(in.Body)
	if in.Type == "" {
		in.Type = "other"
	}
	if err := checkLen("title", in.Title, 1, maxTitle); err != nil {
		return in, err
	}
	if err := checkLen("summary", in.Summary, 0, maxSummary); err != nil {
		return in, err
	}
	if err := checkLen("body", in.Body, 0, maxBody); err != nil {
		return in, err
	}
	if !slices.Contains(Types, in.Type) {
		return in, apperr.Invalid("type", "must be one of %s", strings.Join(Types, ", "))
	}
	tags, err := normalizeTags(in.Tags)
	if err != nil {
		return in, err
	}
	in.Tags = tags
	if err := checkSecrets(in.Title, in.Summary, in.Body, in.Tags); err != nil {
		return in, err
	}
	return in, nil
}

// checkSecrets refuses any field that looks like it holds a credential.
func checkSecrets(title, summary, body string, tags []string) error {
	for _, f := range [][2]string{{"title", title}, {"summary", summary}, {"body", body}, {"tags", strings.Join(tags, " ")}} {
		if kind := DetectSecret(f[1]); kind != "" {
			return apperr.Invalid(f[0], "looks like it contains a %s; credentials and tokens are never stored in memory. Leave the secret out and save the rest", kind)
		}
	}
	return nil
}

// normalizeTags lowercases, trims, dedupes and caps tags. Never nil.
func normalizeTags(tags []string) ([]string, error) {
	out := []string{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")))
		if t == "" || slices.Contains(out, t) {
			continue
		}
		if utf8.RuneCountInString(t) > maxTag {
			return nil, apperr.Invalid("tags", "%q is longer than %d characters", t, maxTag)
		}
		out = append(out, t)
	}
	if len(out) > maxTags {
		return nil, apperr.Invalid("tags", "at most %d tags", maxTags)
	}
	return out, nil
}

func mergeTags(old, add []string) []string {
	out := slices.Clone(old)
	for _, t := range add {
		if !slices.Contains(out, t) && len(out) < maxTags {
			out = append(out, t)
		}
	}
	return out
}

// truncate cuts s to at most n runes.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func checkLen(field, v string, lo, hi int) error {
	if n := utf8.RuneCountInString(v); n < lo || n > hi {
		if lo > 0 {
			return apperr.Invalid(field, "must be %d–%d characters", lo, hi)
		}
		return apperr.Invalid(field, "must be at most %d characters", hi)
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.ErrNotFound
	}
	return err
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefTags(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}
