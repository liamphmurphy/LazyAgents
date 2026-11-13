package artifacts

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
	case "vesper_id", "lazyagents_id", "created", "updated", "project", "title":
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
		other, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: a.WorkspaceID, Path: name})
		if err == nil && other.ID != a.ID {
			return apperr.Invalid("aliases", "%q is another page's path", name)
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		other, err = q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: a.WorkspaceID, Path: name})
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

// DerivedNotePath serializes allocation so concurrent duplicate titles get
// deterministic numeric suffixes, without displacing old moved paths.
func DerivedNotePath(ctx context.Context, q *store.Queries, ws uuid.UUID, title string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", apperr.Invalid("title", "is required when path is omitted")
	}
	if err := q.LockNotePaths(ctx, ws); err != nil {
		return "", err
	}
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
	if utf8.RuneCountInString(base) > 280 {
		base = string([]rune(base)[:280])
	}
	for n := 1; ; n++ {
		p := base
		if n > 1 {
			p = fmt.Sprintf("%s %d", base, n)
		}
		if _, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: p}); err == nil {
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if _, err := q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: ws, Path: p}); err == nil {
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		return ValidNotePath(p)
	}
}
