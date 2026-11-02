// Package artifacts is the domain service for PRDs, specs, ADRs, reports and
// designs: versioned markdown in Postgres, binary files in object storage,
// outlines extracted from headings, and agent read tracking (PLAN.md §4).
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

var Types = []string{"prd", "spec", "adr", "report", "design"}

const (
	// MaxMarkdownBytes bounds text bodies kept in Postgres.
	MaxMarkdownBytes = 2 << 20
	// MaxFileBytes bounds uploads to object storage.
	MaxFileBytes = 25 << 20
)

// Blobs is the subset of blob.Store the service needs.
type Blobs interface {
	Put(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, bucket, key string) error
}

type Service struct {
	Pool   *pgxpool.Pool
	Blobs  Blobs
	Bucket string
}

// Actor is the author of an artifact or version.
type Actor struct {
	Type string // user | agent
	ID   uuid.UUID
}

// File is an uploaded file, already spooled so its size is known.
type File struct {
	Name        string
	ContentType string
	Size        int64
	Body        io.Reader
}

// Content is exactly one of a markdown body or a file.
type Content struct {
	BodyMD *string
	File   *File
}

type NewArtifact struct {
	ProjectKey   string
	Type         string
	Title        string
	Summary      string
	EpicID       *uuid.UUID
	VersionLabel string
	Content      Content
	// Number keeps an existing ADR's number on import; nil takes the next one.
	Number *int32
}

type NewVersion struct {
	Label   string
	Note    string
	Content Content
}

// IsMarkdown reports whether an upload should be stored as text.
func IsMarkdown(name, contentType string) bool {
	ext := strings.ToLower(path.Ext(name))
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return ext == ".md" || ext == ".markdown" || ct == "text/markdown" || ct == "text/x-markdown"
}

// resolved is content ready to store: markdown text or an uploaded blob.
type resolved struct {
	body        *string
	blobKey     *string
	filename    *string
	contentType string
	size        int64
}

// resolve validates content, reads markdown files into text and uploads other
// files. The returned cleanup deletes an uploaded blob if the caller's
// transaction fails.
func (s *Service) resolve(ctx context.Context, ws uuid.UUID, c Content) (resolved, func(), error) {
	noop := func() {}
	if (c.BodyMD == nil) == (c.File == nil) {
		return resolved{}, noop, apperr.Invalid("content", "send exactly one of body_md or file")
	}
	if c.File != nil && IsMarkdown(c.File.Name, c.File.ContentType) {
		if c.File.Size > MaxMarkdownBytes {
			return resolved{}, noop, apperr.Invalid("file", "markdown files are limited to 2 MB")
		}
		b, err := io.ReadAll(io.LimitReader(c.File.Body, MaxMarkdownBytes+1))
		if err != nil {
			return resolved{}, noop, err
		}
		text := string(b)
		c = Content{BodyMD: &text}
	}
	if c.BodyMD != nil {
		body := strings.ReplaceAll(*c.BodyMD, "\r\n", "\n")
		if strings.TrimSpace(body) == "" {
			return resolved{}, noop, apperr.Invalid("body_md", "is empty")
		}
		if len(body) > MaxMarkdownBytes {
			return resolved{}, noop, apperr.Invalid("body_md", "is limited to 2 MB")
		}
		if !utf8.ValidString(body) {
			return resolved{}, noop, apperr.Invalid("body_md", "must be UTF-8 text")
		}
		return resolved{body: &body, contentType: "text/markdown", size: int64(len(body))}, noop, nil
	}

	f := c.File
	if f.Size <= 0 {
		return resolved{}, noop, apperr.Invalid("file", "is empty")
	}
	if f.Size > MaxFileBytes {
		return resolved{}, noop, apperr.Invalid("file", "files are limited to 25 MB")
	}
	if s.Blobs == nil {
		return resolved{}, noop, errors.New("artifacts: no blob store configured")
	}
	name := safeFilename(f.Name)
	key := fmt.Sprintf("%s/%s/%s", ws, uuid.New(), name)
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	if err := s.Blobs.Put(ctx, s.Bucket, key, f.Body, f.Size, ct); err != nil {
		return resolved{}, noop, err
	}
	cleanup := func() { _ = s.Blobs.Delete(context.Background(), s.Bucket, key) }
	return resolved{blobKey: &key, filename: &name, contentType: ct, size: f.Size}, cleanup, nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Trim(unsafeName.ReplaceAllString(name, "-"), "-.")
	if name == "" {
		return "file"
	}
	if len(name) > 120 {
		ext := path.Ext(name)
		name = name[:120-len(ext)] + ext
	}
	return name
}

func defaultTitle(r resolved, given string, fileName string) string {
	if t := strings.TrimSpace(given); t != "" {
		return t
	}
	if r.body != nil {
		for _, s := range Outline(*r.body) {
			if s.Level == 1 {
				return s.Heading
			}
		}
	}
	if fileName != "" {
		return strings.TrimSuffix(path.Base(fileName), path.Ext(fileName))
	}
	return ""
}

func validTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return "", apperr.Invalid("title", "is required (or start the document with a # heading)")
	}
	if utf8.RuneCountInString(t) > 300 {
		return "", apperr.Invalid("title", "must be at most 300 characters")
	}
	return t, nil
}

// writeVersion inserts a version, extracts its outline and makes it current.
func writeVersion(ctx context.Context, q *store.Queries, a store.Artifact, r resolved, label, note string, by Actor, summary string) (store.ArtifactVersion, error) {
	v, err := q.CreateArtifactVersion(ctx, store.CreateArtifactVersionParams{
		ArtifactID: a.ID, Label: label, BodyMd: r.body, BlobKey: r.blobKey, Filename: r.filename,
		ContentType: r.contentType, SizeBytes: r.size, Note: note, CreatedByType: by.Type, CreatedByID: by.ID,
	})
	if err != nil {
		return v, err
	}
	search := ""
	if r.body != nil {
		for i, sec := range Outline(*r.body) {
			if err := q.InsertSection(ctx, store.InsertSectionParams{
				VersionID: v.ID, Position: int32(i + 1), Level: int32(sec.Level), Anchor: sec.Anchor, Heading: sec.Heading,
			}); err != nil {
				return v, err
			}
		}
		search = PlainText(*r.body)
		if summary == "" {
			summary = Summary(*r.body)
		}
	} else if r.filename != nil {
		search = *r.filename
	}
	return v, q.SetArtifactCurrentVersion(ctx, store.SetArtifactCurrentVersionParams{
		ID: a.ID, CurrentVersionID: &v.ID, SearchText: search, Summary: summary,
	})
}

// Create stores a new artifact and its first version.
func (s *Service) Create(ctx context.Context, ws uuid.UUID, by Actor, in NewArtifact) (store.Artifact, error) {
	if !slices.Contains(Types, in.Type) {
		return store.Artifact{}, apperr.Invalid("type", "must be one of %s", strings.Join(Types, ", "))
	}
	fileName := ""
	if in.Content.File != nil {
		fileName = in.Content.File.Name
	}
	r, cleanup, err := s.resolve(ctx, ws, in.Content)
	if err != nil {
		return store.Artifact{}, err
	}
	title, err := validTitle(defaultTitle(r, in.Title, fileName))
	if err != nil {
		cleanup()
		return store.Artifact{}, err
	}
	label := strings.TrimSpace(in.VersionLabel)
	if label == "" {
		label = "v1"
	}

	var a store.Artifact
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.GetProjectByKey(ctx, store.GetProjectByKeyParams{WorkspaceID: ws, Key: in.ProjectKey})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("project_key", "unknown project")
		} else if err != nil {
			return err
		}
		if in.EpicID != nil {
			e, err := q.GetEpic(ctx, store.GetEpicParams{ID: *in.EpicID, WorkspaceID: ws})
			if err != nil || e.ProjectID != p.ID {
				return apperr.Invalid("epic_id", "unknown epic in this project")
			}
		}
		var number *int32
		if in.Number != nil && in.Type != "adr" {
			return apperr.Invalid("number", "only ADRs are numbered")
		}
		if in.Type == "adr" {
			if err := q.LockArtifactNumbering(ctx, ws); err != nil {
				return err
			}
			if in.Number != nil {
				if *in.Number < 1 {
					return apperr.Invalid("number", "must be positive")
				}
				taken, err := q.ADRNumberTaken(ctx, store.ADRNumberTakenParams{WorkspaceID: ws, Number: in.Number})
				if err != nil {
					return err
				}
				if taken {
					return apperr.Invalid("number", "ADR-%03d already exists", *in.Number)
				}
				number = in.Number
			} else {
				n, err := q.NextADRNumber(ctx, ws)
				if err != nil {
					return err
				}
				number = &n
			}
		}
		a, err = q.CreateArtifact(ctx, store.CreateArtifactParams{
			WorkspaceID: ws, ProjectID: p.ID, EpicID: in.EpicID, Type: in.Type, Title: title, Number: number,
			Summary: strings.TrimSpace(in.Summary), AuthorType: by.Type, AuthorID: by.ID,
		})
		if err != nil {
			return err
		}
		_, err = writeVersion(ctx, q, a, r, label, "", by, a.Summary)
		return err
	})
	if err != nil {
		cleanup()
		return a, err
	}
	return a, nil
}

// AddVersion makes new content current. Labels default to v<seq>.
func (s *Service) AddVersion(ctx context.Context, ws uuid.UUID, by Actor, id uuid.UUID, in NewVersion) (store.Artifact, error) {
	r, cleanup, err := s.resolve(ctx, ws, in.Content)
	if err != nil {
		return store.Artifact{}, err
	}
	var a store.Artifact
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		if a, err = q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.ErrNotFound
			}
			return err
		}
		label := strings.TrimSpace(in.Label)
		if label == "" {
			versions, err := q.ListArtifactVersions(ctx, a.ID)
			if err != nil {
				return err
			}
			label = fmt.Sprintf("v%d", len(versions)+1)
		}
		_, err = writeVersion(ctx, q, a, r, label, strings.TrimSpace(in.Note), by, a.Summary)
		return err
	})
	if err != nil {
		cleanup()
	}
	return a, err
}

type MetaPatch struct {
	Title   *string
	Summary *string
	EpicSet bool
	EpicID  *uuid.UUID
}

func (s *Service) UpdateMeta(ctx context.Context, ws uuid.UUID, id uuid.UUID, p MetaPatch) (store.Artifact, error) {
	var a store.Artifact
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		next := store.UpdateArtifactMetaParams{ID: cur.ID, Title: cur.Title, Summary: cur.Summary, EpicID: cur.EpicID}
		if p.Title != nil {
			if next.Title, err = validTitle(*p.Title); err != nil {
				return err
			}
		}
		if p.Summary != nil {
			next.Summary = strings.TrimSpace(*p.Summary)
		}
		if p.EpicSet {
			if p.EpicID != nil {
				e, err := q.GetEpic(ctx, store.GetEpicParams{ID: *p.EpicID, WorkspaceID: ws})
				if err != nil || e.ProjectID != cur.ProjectID {
					return apperr.Invalid("epic_id", "unknown epic in this project")
				}
			}
			next.EpicID = p.EpicID
		}
		a, err = q.UpdateArtifactMeta(ctx, next)
		return err
	})
	return a, err
}

// RecordRead logs an agent reading the current version, optionally one
// section by anchor. The Agent API (Phase 4) calls this from read_artifact.
func RecordRead(ctx context.Context, q *store.Queries, a store.Artifact, agentID uuid.UUID, runID *uuid.UUID, anchor string, at *time.Time) error {
	if a.CurrentVersionID == nil {
		return apperr.Invalid("artifact", "has no content")
	}
	var section *uuid.UUID
	if anchor != "" {
		sec, err := q.GetSectionByAnchor(ctx, store.GetSectionByAnchorParams{VersionID: *a.CurrentVersionID, Anchor: anchor})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Invalid("section", "no section %q in this version", anchor)
		} else if err != nil {
			return err
		}
		section = &sec.ID
	}
	return q.InsertArtifactRead(ctx, store.InsertArtifactReadParams{
		VersionID: *a.CurrentVersionID, SectionID: section, AgentID: agentID, RunID: runID, ReadAt: at,
	})
}

// DisplayID is the human ID for numbered types: ADR-012.
func DisplayID(typ string, number *int32) *string {
	if typ != "adr" || number == nil {
		return nil
	}
	s := fmt.Sprintf("ADR-%03d", *number)
	return &s
}
