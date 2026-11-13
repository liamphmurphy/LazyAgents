package artifacts

import "github.com/liamphmurphy/vesper/server/internal/pageview"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Properties are native frontmatter values: text (including dates and [[links]]),
// numbers, checkboxes and lists. tags and aliases are reserved string lists.
type Properties map[string]any

func DecodeProperties(raw json.RawMessage) Properties {
	p := Properties{}
	_ = json.Unmarshal(raw, &p)
	return p
}

func StringList(v any) ([]string, error) {
	switch v := v.(type) {
	case nil:
		return []string{}, nil
	case string:
		return []string{v}, nil
	case []string:
		return v, nil
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, apperr.Invalid("properties", "lists must contain strings")
			}
			out[i] = s
		}
		return out, nil
	}
	return nil, apperr.Invalid("properties", "tags and aliases must be string lists")
}

// ReservedProperty names are export envelope fields, not custom properties.
func ReservedProperty(k string) bool {
	switch k {
	case "vesper_id", "lazyagents_id", "created", "updated", "project", "title", "audience":
		return true
	}
	return false
}

func NormalizeProperties(p Properties) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	out := Properties{}
	for k, v := range p {
		if ReservedProperty(k) {
			return nil, apperr.Invalid("properties", "%s is reserved frontmatter", k)
		}
		if strings.TrimSpace(k) == "" {
			return nil, apperr.Invalid("properties", "keys must not be empty")
		}
		if k == "tags" || k == "aliases" {
			ss, err := StringList(v)
			if err != nil {
				return nil, err
			}
			vals := []string{}
			for _, s := range ss {
				if k == "tags" {
					s = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "#"))
				} else {
					var err error
					s, err = ValidNotePath(s)
					if err != nil {
						return nil, apperr.Invalid("aliases", "must be link-safe names: %v", err)
					}
				}
				if s != "" && !slices.Contains(vals, s) {
					vals = append(vals, s)
				}
			}
			out[k] = vals
			continue
		}
		switch v.(type) {
		case string, bool, float64, int, int64, json.Number:
			out[k] = v
		case []string, []any:
			ss, err := StringList(v)
			if err != nil {
				return nil, err
			}
			out[k] = ss
		default:
			return nil, apperr.Invalid("properties", "%s must be text, number, date, checkbox, list or link", k)
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, apperr.Invalid("properties", "%v", err)
	}
	if len(b) > MaxMarkdownBytes {
		return nil, apperr.Invalid("properties", "must be at most 2 MB")
	}
	return b, nil
}

var tagCode = regexp.MustCompile("(?s)`[^`\\n]*`|!?\\[\\[[^\\]\\n]*\\]\\]|\\[[^\\]\\n]*\\]\\([^\\n)]*\\)|<[^>\\n]*>")
var inlineTag = regexp.MustCompile(`(?:^|[\s(])#([\pL\pN_][\pL\pN_/-]*)`)

var tagFence = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

func outsideFences(body string) string {
	lines := strings.Split(body, "\n")
	marker := ""
	for i, line := range lines {
		m := tagFence.FindStringSubmatch(line)
		if marker != "" {
			if m != nil && m[1][0] == marker[0] && len(m[1]) >= len(marker) && strings.TrimSpace(m[2]) == "" {
				marker = ""
			}
			lines[i] = ""
			continue
		}
		if m != nil {
			marker = m[1]
			lines[i] = ""
			continue
		}
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func Tags(p Properties, body string) []string {
	tags, _ := StringList(p["tags"])
	tags = append([]string{}, tags...)
	for _, m := range inlineTag.FindAllStringSubmatch(tagCode.ReplaceAllString(outsideFences(body), " "), -1) {
		tags = append(tags, m[1])
	}
	out := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}

// IndexProperties runs only on publication, in the same locked transaction as
// the body. It never exposes properties on a pending suggestion.
func IndexProperties(ctx context.Context, q *store.Queries, a store.Artifact, v store.ArtifactVersion) error {
	if a.Type != "note" {
		return nil
	}
	if err := q.LockNotePaths(ctx, a.WorkspaceID); err != nil {
		return err
	}
	p := DecodeProperties(v.Properties)
	if err := q.DeletePropertyAliases(ctx, a.ID); err != nil {
		return err
	}
	aliases, _ := StringList(p["aliases"])
	for _, name := range aliases {
		other, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: a.WorkspaceID, Path: name, Viewer: pageview.From(ctx).UserID})
		if err == nil && other.ID != a.ID {
			return apperr.Invalid("aliases", "%q is another page's path", name)
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		other, err = q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: a.WorkspaceID, Path: name, Viewer: pageview.From(ctx).UserID})
		if err == nil && other.ID != a.ID {
			return apperr.Invalid("aliases", "%q belongs to another page", name)
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := q.AddPropertyAlias(ctx, store.AddPropertyAliasParams{WorkspaceID: a.WorkspaceID, ArtifactID: a.ID, Path: name}); err != nil {
			return err
		}
	}
	if err := q.DeletePageTags(ctx, a.ID); err != nil {
		return err
	}
	body := ""
	if v.BodyMd != nil {
		body = *v.BodyMd
	}
	return q.AddPageTags(ctx, store.AddPageTagsParams{ArtifactID: a.ID, Tags: Tags(p, body)})
}

// ValidPageTitle follows ADR §3.3: free display text, 1–200 characters.
func ValidPageTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n < 1 || n > 200 {
		return "", apperr.Invalid("title", "must be 1–200 characters")
	}
	return title, nil
}

// DerivedNotePath serializes allocation so concurrent duplicate titles get
// deterministic numeric suffixes, without displacing old moved paths.
func DerivedNotePath(ctx context.Context, q *store.Queries, ws uuid.UUID, title string) (string, error) {
	return derivedNotePath(ctx, q, ws, title, "", uuid.Nil)
}

func derivedNotePath(ctx context.Context, q *store.Queries, ws uuid.UUID, title, folder string, owner uuid.UUID) (string, error) {
	title, err := ValidPageTitle(title)
	if err != nil {
		return "", err
	}
	if err := q.LockNotePaths(ctx, ws); err != nil {
		return "", err
	}
	base := NoteSegment(title)
	budget := 300 - utf8.RuneCountInString(folder)
	for n := 1; ; n++ {
		suffix := ""
		if n > 1 {
			suffix = fmt.Sprintf(" %d", n)
		}
		limit := budget - utf8.RuneCountInString(suffix)
		if limit < 1 {
			return "", apperr.Invalid("path", "folder leaves no room for a unique page name")
		}
		segment := []rune(base)
		if len(segment) > limit {
			segment = segment[:limit]
		}
		p, err := ValidNotePath(folder + strings.TrimSpace(string(segment)) + suffix)
		if err != nil {
			return "", err
		}
		if a, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: p, Viewer: pageview.From(ctx).UserID}); err == nil {
			if a.ID != owner {
				continue
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if a, err := q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: ws, Path: p, Viewer: pageview.From(ctx).UserID}); err == nil {
			if a.ID != owner {
				continue
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		return p, nil
	}
}

// RenameNoteTitleTx changes the title and last path segment together. The
// caller holds the namespace lock before locking the page, as on a folder move.
func RenameNoteTitleTx(ctx context.Context, q *store.Queries, a store.Artifact, title string) (store.Artifact, error) {
	title, err := ValidPageTitle(title)
	if err != nil {
		return a, err
	}
	if title == a.Title {
		return a, nil
	}
	folder := ""
	if i := strings.LastIndex(*a.Path, "/"); i >= 0 {
		folder = (*a.Path)[:i+1]
	}
	p, err := derivedNotePath(ctx, q, a.WorkspaceID, title, folder, a.ID)
	if err != nil {
		return a, err
	}
	if !strings.EqualFold(*a.Path, p) {
		if err := q.AddAlias(ctx, store.AddAliasParams{WorkspaceID: a.WorkspaceID, ArtifactID: a.ID, Path: *a.Path}); err != nil {
			return a, err
		}
	}
	// Allocation permits only this page's aliases; another page's are skipped.
	if err := q.DeleteAlias(ctx, store.DeleteAliasParams{WorkspaceID: a.WorkspaceID, ArtifactID: a.ID, Path: p}); err != nil {
		return a, err
	}
	a, err = q.SetNotePath(ctx, store.SetNotePathParams{ID: a.ID, Path: &p, Title: title})
	if err != nil {
		return a, err
	}
	return a, links.ResolvePending(ctx, q, a.WorkspaceID)
}

// NoteSegment derives the link-safe part of a free display title.
func NoteSegment(title string) string {
	base := strings.Map(func(r rune) rune {
		if strings.ContainsRune("[]#|^/\\", r) || r < 32 {
			return ' '
		}
		return r
	}, title)
	base = strings.Join(strings.Fields(base), " ")
	// Normalize suffixes before checking collisions; ValidNotePath strips .md.
	for strings.HasSuffix(base, ".md") {
		base = strings.TrimSuffix(base, ".md")
	}
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		base = "Untitled"
	}
	return base
}

func DerivedNotePathInFolder(ctx context.Context, q *store.Queries, ws uuid.UUID, title, folder string) (string, error) {
	folder = strings.Trim(folder, "/")
	if folder != "" {
		if _, err := ValidNotePath(folder + "/placeholder"); err != nil {
			return "", err
		}
		folder += "/"
	}
	return derivedNotePath(ctx, q, ws, title, folder, uuid.Nil)
}
