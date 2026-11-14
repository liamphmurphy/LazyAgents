// Package projects manages the work a space tracks: its key, repository settings and
// reversible archiving. Since LAZYAGENTS-213 a project is never on its own: it is the work of
// exactly one space (projects.space_id), made with it or when the space starts tracking work.
package projects

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type Service struct{ Pool *pgxpool.Pool }
type Config struct {
	Name          string `json:"name"`
	Color         string `json:"color"`
	RepoURL       string `json:"repo_url"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	Revision      int32  `json:"revision"`
}

// SpaceColorHex is each space color's hex, the color its project takes. web/lib/spaces.ts and
// nearest_space_color (migration 00132) hold the same values.
var SpaceColorHex = map[string]string{
	"green": "#34D399", "amber": "#FBBF24", "red": "#F87171", "blue": "#60A5FA",
	"violet": "#A78BFA", "pink": "#F472B6", "teal": "#2DD4BF", "orange": "#FB923C",
}

var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var repoPattern = regexp.MustCompile(`^(?:git@github\.com:|ssh://git@github\.com/|https://github\.com/)[A-Za-z0-9][A-Za-z0-9-]{0,99}/[A-Za-z0-9._-]+$`)
var httpsRepoPattern = regexp.MustCompile(`^https://(?:www\.)?github\.com/([A-Za-z0-9][A-Za-z0-9-]{0,99})/([A-Za-z0-9._-]+?)(?:\.git)?$`)
var branchPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]{0,199}$`)

// normalize validates everything, including a strict GitHub repo_url (Create).
func normalize(in *Config) error {
	if err := normalizeBase(in); err != nil {
		return err
	}
	return canonicalRepoURL(&in.RepoURL)
}

// canonicalRepoURL accepts a GitHub repository URL and rejects anything else.
// An SSH URL is kept as-is: runs clone and push it with the git:ssh key. An
// HTTPS link (https://github.com/owner/repo[.git]) is stored as
// https://github.com/owner/repo.git: runs use the git:pat token, which the
// setup script already passes to git over HTTPS. web/lib/repo-url.ts mirrors it.
func canonicalRepoURL(u *string) error {
	if m := httpsRepoPattern.FindStringSubmatch(*u); m != nil {
		*u = "https://github.com/" + m[1] + "/" + m[2] + ".git"
	}
	if *u != "" && (!repoPattern.MatchString(*u) || strings.HasSuffix(*u, "/...git") || strings.HasSuffix(*u, "/..git") || strings.HasSuffix(*u, "/..") || strings.HasSuffix(*u, "/.")) {
		return apperr.Invalid("repo_url", "use a GitHub repository URL like https://github.com/owner/repo or git@github.com:owner/repo.git")
	}
	return nil
}

// normalizeBase trims and validates all fields except repo_url's format.
func normalizeBase(in *Config) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Color = strings.TrimSpace(in.Color)
	in.RepoURL = strings.TrimRight(strings.TrimSpace(in.RepoURL), "/")
	in.DefaultBranch = strings.TrimSpace(in.DefaultBranch)
	if in.Name == "" || len([]rune(in.Name)) > 100 {
		return apperr.Invalid("name", "use a name with 1–100 characters")
	}
	if !colorPattern.MatchString(in.Color) {
		return apperr.Invalid("color", "use a six-digit hex color")
	}
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	if !branchPattern.MatchString(in.DefaultBranch) || strings.Contains(in.DefaultBranch, "..") || strings.Contains(in.DefaultBranch, "//") || strings.Contains(in.DefaultBranch, "/.") || strings.HasSuffix(in.DefaultBranch, "/") || strings.HasSuffix(in.DefaultBranch, ".") || strings.HasSuffix(in.DefaultBranch, ".lock") {
		return apperr.Invalid("default_branch", "use a valid Git branch name")
	}
	for _, part := range strings.Split(in.DefaultBranch, "/") {
		if strings.HasSuffix(part, ".lock") {
			return apperr.Invalid("default_branch", "a branch component cannot end in .lock")
		}
	}
	return nil
}
func repo(in Config) *string {
	if in.RepoURL == "" {
		return nil
	}
	return &in.RepoURL
}

// Create makes a project and its space, owned by owner: the space new chats about the work
// start in, with space memory on as for any new space.
func (s Service) Create(ctx context.Context, ws, owner uuid.UUID, key string, in Config) (store.Project, error) {
	var out store.Project
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := create(ctx, q, ws, key, in)
		if err != nil {
			return err
		}
		sid, err := q.CreateSpaceForProject(ctx, store.CreateSpaceForProjectParams{UserID: owner, SpaceMemory: true, ProjectID: p.ID})
		if err != nil {
			return err
		}
		p.SpaceID = &sid
		out = p
		return nil
	})
	return out, err
}

func create(ctx context.Context, q *store.Queries, ws uuid.UUID, key string, in Config) (store.Project, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if !keyPattern.MatchString(key) {
		return store.Project{}, apperr.Invalid("key", "use 2–10 letters or digits, starting with a letter")
	}
	if err := normalize(&in); err != nil {
		return store.Project{}, err
	}
	p, err := q.CreateConfiguredProject(ctx, store.CreateConfiguredProjectParams{WorkspaceID: ws, Key: key, Name: in.Name, Color: in.Color, RepoUrl: repo(in), DefaultBranch: in.DefaultBranch})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return p, apperr.Conflict("the key %s is already in use", key)
	}
	return p, err
}

// Work is what a space needs to start tracking work.
type Work struct {
	Key           string `json:"key"`
	RepoURL       string `json:"repo_url"`
	DefaultBranch string `json:"default_branch"`
}

// TrackWork makes one of the person's spaces track work: a project with the space's name and
// color, which new chats in the space file tickets into from then on.
func (s Service) TrackWork(ctx context.Context, ws, user, spaceID uuid.UUID, w Work) (store.Project, error) {
	var out store.Project
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = TrackWorkTx(ctx, store.New(tx), ws, user, spaceID, w)
		return err
	})
	return out, err
}

// TrackWorkTx is TrackWork inside the caller's transaction.
func TrackWorkTx(ctx context.Context, q *store.Queries, ws, user, spaceID uuid.UUID, w Work) (store.Project, error) {
	sp, err := q.GetChatSpace(ctx, store.GetChatSpaceParams{ID: spaceID, WorkspaceID: ws, UserID: user})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Project{}, apperr.ErrNotFound
	}
	if err != nil {
		return store.Project{}, err
	}
	if sp.App != nil {
		return store.Project{}, apperr.Invalid("space", "an app's space can't track work")
	}
	if _, err := q.GetProjectBySpace(ctx, &sp.ID); err == nil {
		return store.Project{}, apperr.Conflict("this space already tracks work")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return store.Project{}, err
	}
	p, err := create(ctx, q, ws, w.Key, Config{Name: sp.Name, Color: SpaceColorHex[sp.Color], RepoURL: w.RepoURL, DefaultBranch: w.DefaultBranch})
	if err != nil {
		return p, err
	}
	if err := q.LinkProjectSpace(ctx, store.LinkProjectSpaceParams{ProjectID: &p.ID, SpaceID: sp.ID}); err != nil {
		return p, err
	}
	p.SpaceID = &sp.ID
	return p, nil
}

// StopTrackingWork takes a space's work away: only while it has no tickets, since tickets
// can't move or go. The project is archived, out of every list; its key stays taken.
func (s Service) StopTrackingWork(ctx context.Context, ws, user, spaceID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		sp, err := q.GetChatSpace(ctx, store.GetChatSpaceParams{ID: spaceID, WorkspaceID: ws, UserID: user})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		p, err := q.GetProjectBySpace(ctx, &sp.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("this space doesn't track work")
		}
		if err != nil {
			return err
		}
		has, err := q.ProjectHasTickets(ctx, p.ID)
		if err != nil {
			return err
		}
		if has {
			return apperr.Conflict("%s has tickets, so it keeps tracking work; archive it instead", p.Key)
		}
		live, err := q.ProjectHasLiveRuns(ctx, p.ID)
		if err != nil {
			return err
		}
		if live {
			return apperr.Conflict("finish or cancel active and queued runs first")
		}
		return q.UnlinkProjectSpace(ctx, p.ID)
	})
}

func (s Service) Update(ctx context.Context, ws uuid.UUID, key string, in Config) (store.Project, error) {
	var out store.Project
	if err := normalizeBase(&in); err != nil {
		return out, err
	}
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.LockProjectByKey(ctx, store.LockProjectByKeyParams{WorkspaceID: ws, Key: strings.ToUpper(key)})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.Revision != in.Revision {
			return apperr.Conflict("the space's work changed; reload before saving")
		}
		// An unchanged (possibly legacy) value is kept as-is; a new one must be
		// GitHub, and is stored in canonical form.
		stored := ""
		if p.RepoUrl != nil {
			stored = *p.RepoUrl
		}
		if in.RepoURL != stored {
			if err := canonicalRepoURL(&in.RepoURL); err != nil {
				return err
			}
		}
		if in.Archived && !p.Archived {
			live, err := q.ProjectHasLiveRuns(ctx, p.ID)
			if err != nil {
				return err
			}
			if live {
				return apperr.Conflict("finish or cancel active and queued runs before archiving this project")
			}
		}
		if err := q.FreezeLegacyProjectRepositories(ctx, p.ID); err != nil {
			return err
		}
		out, err = q.UpdateProject(ctx, store.UpdateProjectParams{ID: p.ID, Name: in.Name, Color: in.Color, RepoUrl: repo(in), DefaultBranch: in.DefaultBranch, Archived: in.Archived})
		if err != nil {
			return err
		}
		// The space and its work are one thing: a rename or recolor here shows on the space.
		return q.SyncSpaceFromProject(ctx, store.SyncSpaceFromProjectParams{Name: out.Name, Color: out.Color, ProjectID: out.ID})
	})
	return out, err
}
