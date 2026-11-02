package wiki

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Export writes every published note as an Obsidian-compatible vault: one
// <path>.md per page, [[wikilinks]] as written, properties as frontmatter.
func Export(ctx context.Context, q *store.Queries, ws uuid.UUID, w io.Writer) error {
	rows, err := q.ExportNotes(ctx, ws)
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
		fm := fmt.Sprintf("---\nlazyagents_id: %s\ncreated: %s\nupdated: %s\n", r.ID, r.CreatedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339))
		if r.ProjectKey != nil {
			fm += "project: " + *r.ProjectKey + "\n"
		}
		if _, err := io.WriteString(f, fm+"---\n\n"+*r.BodyMd); err != nil {
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
// its path. Frontmatter is dropped except `project`, used for new pages.
// Everything is one transaction, so a bad file leaves the wiki untouched.
func (s *Service) Import(ctx context.Context, ws, userID uuid.UUID, data []byte) (ImportResult, error) {
	res := ImportResult{Skipped: []string{}}
	if len(data) > maxVaultBytes {
		return res, apperr.Invalid("file", "vaults are limited to 50 MB")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return res, apperr.Invalid("file", "isn't a zip file")
	}
	err = s.tx(ctx, func(q *store.Queries) error {
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
			body, props := splitFrontmatter(string(b))
			if strings.TrimSpace(body) == "" {
				res.Skipped = append(res.Skipped, name)
				continue
			}
			p := strings.TrimSuffix(name, path.Ext(name))
			r, err := s.write(ctx, q, ws, artifacts.Actor{Type: "user", ID: userID}, Save{Path: p, Body: body, ProjectKey: props["project"], Note: "Imported"}, artifacts.Published)
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

// splitFrontmatter strips a leading YAML block and returns its simple
// key: value pairs.
func splitFrontmatter(md string) (string, map[string]string) {
	props := map[string]string{}
	md = strings.TrimPrefix(strings.ReplaceAll(md, "\r\n", "\n"), "\ufeff")
	if !strings.HasPrefix(md, "---\n") {
		return md, props
	}
	end := strings.Index(md[4:], "\n---")
	if end < 0 {
		return md, props
	}
	for _, line := range strings.Split(md[4:4+end], "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			props[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	rest := md[4+end+4:]
	return strings.TrimLeft(rest, "\n"), props
}

func asValidation(err error, target **apperr.ValidationError) bool { return errors.As(err, target) }
