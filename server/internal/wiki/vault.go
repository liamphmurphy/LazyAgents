package wiki

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Export writes every published note as an Obsidian-compatible vault: one
// <path>.md per page, [[wikilinks]] as written, properties as frontmatter.
func Export(ctx context.Context, q *store.Queries, ws uuid.UUID, w io.Writer) error {
	rows, err := q.ExportNotes(ctx, store.ExportNotesParams{WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
	if err != nil {
		return err
	}
	z := zip.NewWriter(w)
	for _, r := range rows {
		if r.Path == nil || r.BodyMd == nil {
			continue
		}
		f, err := z.CreateHeader(&zip.FileHeader{Name: *r.Path + ".md", Method: zip.Deflate, Modified: r.UpdatedAt})
		if err != nil {
			return err
		}
		props := artifacts.DecodeProperties(r.Properties)
		fm := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		add := func(k string, v any) error {
			var n yaml.Node
			if err := n.Encode(v); err != nil {
				return err
			}
			fm.Content = append(fm.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, &n)
			return nil
		}
		for _, kv := range [][2]string{{"vesper_id", r.ID.String()}, {"created", r.CreatedAt.Format(time.RFC3339)}, {"updated", r.UpdatedAt.Format(time.RFC3339)}, {"title", r.Title}} {
			if err := add(kv[0], kv[1]); err != nil {
				return err
			}
		}
		if r.ProjectKey != nil {
			if err := add("project", *r.ProjectKey); err != nil {
				return err
			}
		}
		keys := []string{}
		for k := range props {
			if !reservedFrontmatter(k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := add(k, props[k]); err != nil {
				return err
			}
		}
		b, err := yaml.Marshal(fm)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, "---\n"+string(b)+"---\n\n"+*r.BodyMd); err != nil {
			return err
		}
	}
	return z.Close()
}

// ImportResult counts what an import did.
type ImportResult struct {
	Created   int      `json:"created"`
	Updated   int      `json:"updated"`
	Unchanged int      `json:"unchanged"`
	Skipped   []string `json:"skipped"`
}

const maxVaultBytes = 50 << 20

// Import reads a vault zip: each .md file becomes (or replaces) the note at
// its path. Frontmatter is stored with the body on the published version.
// Everything is one transaction, so a bad file leaves the wiki untouched.
func (s *Service) Import(ctx context.Context, ws, userID uuid.UUID, data []byte) (ImportResult, error) {
	ctx = pageview.Person(ctx, userID)
	res := ImportResult{Skipped: []string{}}
	if len(data) > maxVaultBytes {
		return res, apperr.Invalid("file", "vaults are limited to 50 MB")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return res, apperr.Invalid("file", "isn't a zip file")
	}
	err = s.tx(ctx, ws, func(q *store.Queries) error {
		for _, f := range z.File {
			name := strings.ReplaceAll(f.Name, "\\", "/")
			base := path.Base(name)
			if f.FileInfo().IsDir() || strings.HasPrefix(base, ".") || strings.Contains(name, "/.") || strings.HasPrefix(name, "__MACOSX/") {
				continue
			}
			if !strings.EqualFold(path.Ext(name), ".md") {
				res.Skipped = append(res.Skipped, name)
				continue
			}
			if f.UncompressedSize64 > artifacts.MaxMarkdownBytes {
				return apperr.Invalid("file", "%s is larger than 2 MB", name)
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			b, err := io.ReadAll(io.LimitReader(rc, artifacts.MaxMarkdownBytes+1))
			rc.Close()
			if err != nil {
				return err
			}
			body, props, err := splitFrontmatter(string(b))
			if err != nil {
				return apperr.Invalid("file", "%s: invalid frontmatter: %v", name, err)
			}
			project, _ := props["project"].(string)
			title, _ := props["title"].(string)
			var displayTitle *string
			if title != "" {
				displayTitle = &title
			}
			for k := range props {
				if reservedFrontmatter(k) {
					delete(props, k)
				}
			}
			if strings.TrimSpace(body) == "" {
				res.Skipped = append(res.Skipped, name)
				continue
			}
			p := strings.TrimSuffix(name, path.Ext(name))
			r, err := s.write(ctx, q, ws, artifacts.Actor{Type: "user", ID: userID}, Save{Path: p, Body: body, Title: displayTitle, Properties: props, ProjectKey: project, Note: "Imported"}, artifacts.Published)
			if err != nil {
				var ve *apperr.ValidationError
				if asValidation(err, &ve) {
					return apperr.Invalid("file", "%s: %s", name, ve.Error())
				}
				return err
			}
			switch {
			case r.Created:
				res.Created++
			case r.Version.ID == uuid.Nil:
				res.Unchanged++
			default:
				res.Updated++
			}
		}
		return nil
	})
	return res, err
}

func reservedFrontmatter(k string) bool { return artifacts.ReservedProperty(k) }

// YAML's timestamps stay strings; unknown nested maps are retained as YAML
// text rather than being silently dropped. Malformed frontmatter aborts the
// transaction, like any other validation error.
func splitFrontmatter(md string) (string, artifacts.Properties, error) {
	props := artifacts.Properties{}
	md = strings.TrimPrefix(strings.ReplaceAll(md, "\r\n", "\n"), "\ufeff")
	if !strings.HasPrefix(md, "---\n") {
		return md, props, nil
	}
	lines := strings.SplitAfter(md[4:], "\n")
	offset := 4
	end := -1
	for _, line := range lines {
		if strings.TrimRight(line, " \t\r\n") == "---" {
			end = offset
			offset += len(line)
			break
		}
		offset += len(line)
	}
	if end < 0 {
		return "", nil, apperr.Invalid("properties", "unclosed YAML block")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(md[4:end]), &doc); err != nil {
		return "", nil, err
	}
	if len(doc.Content) > 0 {
		root := doc.Content[0]
		if root.Kind != yaml.MappingNode {
			return "", nil, apperr.Invalid("properties", "must be a mapping")
		}
		seen := map[string]bool{}
		for i := 0; i < len(root.Content); i += 2 {
			key, n := root.Content[i], root.Content[i+1]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return "", nil, apperr.Invalid("properties", "keys must be unique strings")
			}
			seen[key.Value] = true
			var val any
			switch {
			case n.Kind == yaml.MappingNode:
				raw, err := yaml.Marshal(n)
				if err != nil {
					return "", nil, err
				}
				val = string(raw)
			case n.Tag == "!!timestamp":
				val = n.Value
			default:
				if err := n.Decode(&val); err != nil {
					return "", nil, err
				}
			}
			props[key.Value] = val
		}
	}
	return strings.TrimPrefix(md[offset:], "\n"), props, nil
}

func asValidation(err error, target **apperr.ValidationError) bool { return errors.As(err, target) }
