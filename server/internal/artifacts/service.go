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
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

var Types = []string{"prd", "spec", "adr", "report", "design", "note", "mockup"}

// Version states. Suggested versions are agent edits to shared notes waiting
// for a person (PLAN.md §12.5); they never become current on their own.
const (
	Published = "published"
	Suggested = "suggested"
	Rejected  = "rejected"
)

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
	ProjectKey   string // required except for notes
	Path         string // notes only: "Payments/Ledger/Rollback runbook"
	Type         string
	Title        string
	Summary      string
	EpicID       *uuid.UUID
	VersionLabel string
	Content      Content
	// Number keeps an existing ADR's number on import; nil takes the next one.
	Number *int32
	// State is Suggested for a note an agent proposes but may not publish:
	// it has no current version until a person accepts it.
	State string
}

type NewVersion struct {
	Label   string
	Note    string
	Content Content
	// State defaults to published. A suggested version is stored without
	// becoming current; BasedOn records the version it was written against.
	State   string
	BasedOn *uuid.UUID
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
func (s *Service) resolve(ctx context.Context, ws uuid.UUID, typ string, c Content) (resolved, func(), error) {
	noop := func() {}
	if (c.BodyMD == nil) == (c.File == nil) {
		return resolved{}, noop, apperr.Invalid("content", "send exactly one of body_md or file")
	}
	mockup := typ == "mockup"
	if mockup && c.File != nil && !IsSVG(c.File.Name, c.File.ContentType) {
		return resolved{}, noop, apperr.Invalid("file", "mockups are SVG files")
	}
	if c.File != nil && (mockup || IsMarkdown(c.File.Name, c.File.ContentType)) {
		if c.File.Size > MaxMarkdownBytes {
			return resolved{}, noop, apperr.Invalid("file", "text files are limited to 2 MB")
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
		if mockup {
			if err := ValidSVG(body); err != nil {
				return resolved{}, noop, err
			}
			return resolved{body: &body, contentType: MockupContentType, size: int64(len(body))}, noop, nil
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
	if r.body != nil && r.contentType != MockupContentType {
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

// writeVersion inserts a version and extracts its outline. Published
// versions become current and have their [[links]] indexed.
func writeVersion(ctx context.Context, q *store.Queries, a store.Artifact, r resolved, label, note string, by Actor, summary, state string, basedOn *uuid.UUID) (store.ArtifactVersion, error) {
	if state == "" {
		state = Published
	}
	var review *string
	if a.Type == "mockup" {
		pending := ReviewPending
		review = &pending
	}
	v, err := q.CreateArtifactVersion(ctx, store.CreateArtifactVersionParams{
		ArtifactID: a.ID, Label: label, BodyMd: r.body, BlobKey: r.blobKey, Filename: r.filename,
		ContentType: r.contentType, SizeBytes: r.size, Note: note, CreatedByType: by.Type, CreatedByID: by.ID,
		State: state, BasedOnVersionID: basedOn, ReviewState: review,
	})
	if err != nil {
		return v, err
	}
	if r.body != nil && a.Type != "mockup" {
		for i, sec := range Outline(*r.body) {
			if err := q.InsertSection(ctx, store.InsertSectionParams{
				VersionID: v.ID, Position: int32(i + 1), Level: int32(sec.Level), Anchor: sec.Anchor, Heading: sec.Heading,
			}); err != nil {
				return v, err
			}
		}
	}
	if state != Published {
		return v, nil
	}
	return v, makeCurrent(ctx, q, a, v, summary)
}

// makeCurrent points the artifact at a version, refreshes its search text
// and summary, and re-indexes the links it contains.
func makeCurrent(ctx context.Context, q *store.Queries, a store.Artifact, v store.ArtifactVersion, summary string) error {
	search := ""
	mockup := a.Type == "mockup"
	if mockup && v.BodyMd != nil {
		search = SVGText(*v.BodyMd)
	} else if v.BodyMd != nil {
		search = PlainText(*v.BodyMd)
		if summary == "" || a.Type == "note" {
			summary = Summary(*v.BodyMd)
		}
	} else if v.Filename != nil {
		search = *v.Filename
	}
	if err := q.SetArtifactCurrentVersion(ctx, store.SetArtifactCurrentVersionParams{
		ID: a.ID, CurrentVersionID: &v.ID, SearchText: search, Summary: summary,
	}); err != nil {
		return err
	}
	body := ""
	if v.BodyMd != nil && !mockup {
		body = *v.BodyMd
	}
	return links.Index(ctx, q, a.WorkspaceID, links.Source{Kind: links.SourceArtifact, ID: a.ID, VersionID: &v.ID}, body)
}

// Publish makes a stored (suggested) version current. The caller marks the
// version published and holds the artifact lock.
func Publish(ctx context.Context, q *store.Queries, a store.Artifact, v store.ArtifactVersion) error {
	return makeCurrent(ctx, q, a, v, a.Summary)
}

// Create stores a new artifact and its first version.
func (s *Service) Create(ctx context.Context, ws uuid.UUID, by Actor, in NewArtifact) (store.Artifact, error) {
	if !slices.Contains(Types, in.Type) {
		return store.Artifact{}, apperr.Invalid("type", "must be one of %s", strings.Join(Types, ", "))
	}
	if in.Type != "note" && in.Path != "" {
		return store.Artifact{}, apperr.Invalid("path", "only notes have a path")
	}
	if in.Type == "note" && in.Content.File != nil && !IsMarkdown(in.Content.File.Name, in.Content.File.ContentType) {
		return store.Artifact{}, apperr.Invalid("file", "notes are markdown")
	}
	r, cleanup, err := s.resolve(ctx, ws, in.Type, in.Content)
	if err != nil {
		return store.Artifact{}, err
	}
	var a store.Artifact
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		a, err = s.create(ctx, store.New(tx), ws, by, in, r)
		return err
	})
	if err != nil {
		cleanup()
	}
	return a, err
}

// CreateNoteTx creates a markdown note inside the caller's transaction.
func (s *Service) CreateNoteTx(ctx context.Context, q *store.Queries, ws uuid.UUID, by Actor, path, projectKey, body, state string) (store.Artifact, error) {
	in := NewArtifact{Type: "note", Path: path, ProjectKey: projectKey, Content: Content{BodyMD: &body}, State: state}
	r, _, err := s.resolve(ctx, ws, "note", in.Content) // markdown: nothing to clean up
	if err != nil {
		return store.Artifact{}, err
	}
	return s.create(ctx, q, ws, by, in, r)
}

// CreateMockupTx creates a mockup from SVG source inside the caller's transaction.
func (s *Service) CreateMockupTx(ctx context.Context, q *store.Queries, ws uuid.UUID, by Actor, projectKey, title string, epicID *uuid.UUID, svg string) (store.Artifact, error) {
	in := NewArtifact{Type: "mockup", ProjectKey: projectKey, Title: title, EpicID: epicID, Content: Content{BodyMD: &svg}}
	r, _, err := s.resolve(ctx, ws, "mockup", in.Content) // text: nothing to clean up
	if err != nil {
		return store.Artifact{}, err
	}
	return s.create(ctx, q, ws, by, in, r)
}

func (s *Service) create(ctx context.Context, q *store.Queries, ws uuid.UUID, by Actor, in NewArtifact, r resolved) (store.Artifact, error) {
	var a store.Artifact
	fileName := ""
	if in.Content.File != nil {
		fileName = in.Content.File.Name
	}
	var notePath *string
	title := ""
	if in.Type == "note" {
		p, err := ValidNotePath(in.Path)
		if err != nil {
			return a, err
		}
		notePath, title = &p, links.TitleOf(p)
	} else {
		var err error
		if title, err = validTitle(defaultTitle(r, in.Title, fileName)); err != nil {
			return a, err
		}
	}
	label := strings.TrimSpace(in.VersionLabel)
	if label == "" {
		label = "v1"
	}
	var projectID *uuid.UUID
	if in.ProjectKey != "" || in.Type != "note" {
		p, err := q.LockProjectByKey(ctx, store.LockProjectByKeyParams{WorkspaceID: ws, Key: in.ProjectKey})
		if errors.Is(err, pgx.ErrNoRows) {
			return a, apperr.Invalid("project_key", "unknown project")
		} else if err != nil {
			return a, err
		}
		if p.Archived {
			return a, apperr.Conflict("restore the archived project before creating artifacts")
		}
		projectID = &p.ID
	}
	if in.EpicID != nil {
		e, err := q.GetEpic(ctx, store.GetEpicParams{ID: *in.EpicID, WorkspaceID: ws})
		if err != nil || projectID == nil || e.ProjectID != *projectID {
			return a, apperr.Invalid("epic_id", "unknown epic in this project")
		}
	}
	if notePath != nil {
		if _, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: *notePath}); err == nil {
			return a, apperr.Invalid("path", "a page called %q already exists", *notePath)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return a, err
		}
		// The new page takes over this path from any renamed page.
		if err := q.DeleteAlias(ctx, store.DeleteAliasParams{WorkspaceID: ws, Path: *notePath}); err != nil {
			return a, err
		}
	}
	var number *int32
	if in.Number != nil && in.Type != "adr" {
		return a, apperr.Invalid("number", "only ADRs are numbered")
	}
	if in.Type == "adr" {
		if err := q.LockArtifactNumbering(ctx, ws); err != nil {
			return a, err
		}
		if in.Number != nil {
			if *in.Number < 1 {
				return a, apperr.Invalid("number", "must be positive")
			}
			taken, err := q.ADRNumberTaken(ctx, store.ADRNumberTakenParams{WorkspaceID: ws, Number: in.Number})
			if err != nil {
				return a, err
			}
			if taken {
				return a, apperr.Invalid("number", "ADR-%03d already exists", *in.Number)
			}
			number = in.Number
		} else {
			n, err := q.NextADRNumber(ctx, ws)
			if err != nil {
				return a, err
			}
			number = &n
		}
	}
	a, err := q.CreateArtifact(ctx, store.CreateArtifactParams{
		WorkspaceID: ws, ProjectID: projectID, EpicID: in.EpicID, Type: in.Type, Title: title, Number: number,
		Summary: strings.TrimSpace(in.Summary), AuthorType: by.Type, AuthorID: by.ID, Path: notePath,
	})
	if err != nil {
		return a, err
	}
	if in.State == Suggested {
		label = "suggestion"
	}
	if _, err = writeVersion(ctx, q, a, r, label, "", by, a.Summary, in.State, nil); err != nil {
		return a, err
	}
	if in.State == Suggested {
		return a, nil // nothing to resolve to until a person accepts it
	}
	// Ghost links naming this artifact now resolve to it.
	return a, links.ResolvePending(ctx, q, ws)
}

// AddVersion makes new content current. Labels default to v<seq>.
func (s *Service) AddVersion(ctx context.Context, ws uuid.UUID, by Actor, id uuid.UUID, in NewVersion) (store.Artifact, error) {
	cur, err := store.New(s.Pool).GetArtifact(ctx, store.GetArtifactParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Artifact{}, apperr.ErrNotFound
	} else if err != nil {
		return store.Artifact{}, err
	}
	r, cleanup, err := s.resolve(ctx, ws, cur.Type, in.Content)
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
		_, err = AddVersionTx(ctx, q, a, r, by, in)
		return err
	})
	if err != nil {
		cleanup()
	}
	return a, err
}

// AddVersionTx adds a version to a locked artifact in the caller's
// transaction. Labels count published versions: a suggestion accepted later
// is relabelled then.
func AddVersionTx(ctx context.Context, q *store.Queries, a store.Artifact, r resolved, by Actor, in NewVersion) (store.ArtifactVersion, error) {
	if a.Type == "note" && r.body == nil {
		return store.ArtifactVersion{}, apperr.Invalid("file", "notes are markdown")
	}
	if a.Type == "mockup" && (r.body == nil || r.contentType != MockupContentType) {
		return store.ArtifactVersion{}, apperr.Invalid("content", "mockups take SVG text")
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		n, err := q.CountPublishedVersions(ctx, a.ID)
		if err != nil {
			return store.ArtifactVersion{}, err
		}
		label = fmt.Sprintf("v%d", n+1)
		if in.State == Suggested {
			label = "suggestion"
		}
	}
	return writeVersion(ctx, q, a, r, label, strings.TrimSpace(in.Note), by, a.Summary, in.State, in.BasedOn)
}

// Markdown validates a markdown body for AddVersionTx.
func Markdown(body string) (resolved, error) {
	var s Service
	r, _, err := s.resolve(context.Background(), uuid.Nil, "", Content{BodyMD: &body})
	return r, err
}

// Mockup validates SVG source for AddVersionTx.
func Mockup(body string) (resolved, error) {
	var s Service
	r, _, err := s.resolve(context.Background(), uuid.Nil, "mockup", Content{BodyMD: &body})
	return r, err
}

// ValidNotePath normalizes a wiki path and checks it can be linked to.
func ValidNotePath(p string) (string, error) {
	p = links.NormalizePath(p)
	if p == "" {
		return "", apperr.Invalid("path", "is required, e.g. Payments/Rollback runbook")
	}
	if strings.ContainsAny(p, "[]#|^") {
		return "", apperr.Invalid("path", "can't contain [ ] # | or ^, which would break [[links]] to it")
	}
	if utf8.RuneCountInString(p) > 300 {
		return "", apperr.Invalid("path", "must be at most 300 characters")
	}
	if strings.Count(p, "/") > 8 {
		return "", apperr.Invalid("path", "can be at most 9 levels deep")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return "", apperr.Invalid("path", "can't contain . or .. segments")
		}
	}
	return p, nil
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
		if p.Title != nil && cur.Type == "note" && *p.Title != cur.Title {
			return apperr.Invalid("title", "a page's title is the last part of its path: move the page to rename it")
		}
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
				if err != nil || cur.ProjectID == nil || e.ProjectID != *cur.ProjectID {
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
