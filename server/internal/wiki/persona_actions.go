package wiki

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Persona versions of the explorer's destructive actions. Like a move, a
// delete or a filed capture can't wait as a suggestion, so they follow the
// same rule (personaMayMove): only pages whose persona changes apply at once,
// never paused, review-first or agent pages. The person does the rest in the
// Knowledge app.

// DeletePageAsPersona deletes one of the person's pages, with its history, for
// a persona in their chat.
func (s *Service) DeletePageAsPersona(ctx context.Context, ws, userID, id uuid.UUID) (store.Artifact, error) {
	ctx = pageview.Person(ctx, userID)
	var a store.Artifact
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		var err error
		a, err = lockPersonaPage(ctx, q, ws, userID, id)
		if err != nil {
			return err
		}
		if err := personaMay(a, "delete", "deleted"); err != nil {
			return err
		}
		return deletePage(ctx, q, a)
	})
	return a, err
}

// DeleteFolderAsPersona deletes every page under one of the person's folders.
// It deletes nothing unless the persona may delete every page in it.
func (s *Service) DeleteFolderAsPersona(ctx context.Context, ws, userID uuid.UUID, folder string) ([]uuid.UUID, error) {
	src, err := FolderScope{Folder: folder}.clean()
	if err != nil {
		return nil, err
	}
	if err := personaMovePath("folder", src); err != nil {
		return nil, err
	}
	ctx = pageview.Person(ctx, userID)
	deleted := []uuid.UUID{}
	err = s.tx(ctx, ws, func(q *store.Queries) error {
		pages, err := q.LockNotesInFolder(ctx, store.LockNotesInFolderParams{WorkspaceID: ws, Folder: src, Viewer: &userID})
		if err != nil {
			return err
		}
		if len(pages) == 0 {
			return apperr.ErrNotFound
		}
		// Check them all first, so one refused page leaves the folder whole.
		for _, p := range pages {
			if err := personaMay(p, "delete", "deleted"); err != nil {
				return err
			}
		}
		for _, p := range pages {
			if err := deletePage(ctx, q, p); err != nil {
				return err
			}
			deleted = append(deleted, p.ID)
		}
		return nil
	})
	return deleted, err
}

// FileCaptureAsPersona files an inbox capture for a persona: into a folder,
// or merged into the end of another page and then deleted. Both pages must
// take the persona's changes at once, and the merge is the persona's version
// of the target, left for the person to review afterwards.
func (s *Service) FileCaptureAsPersona(ctx context.Context, ws, userID uuid.UUID, agent store.Agent, id uuid.UUID, folder string, merge *uuid.UUID) (Result, error) {
	if merge == nil {
		folder = strings.Trim(strings.TrimSpace(folder), "/")
		if folder == "" {
			return Result{}, apperr.Invalid("folder", "give a folder to file it in, or a page to merge it into")
		}
		p, _, err := OpenPage(pageview.Person(ctx, userID), store.New(s.Pool), ws, id.String())
		if err != nil {
			return Result{}, err
		}
		a, err := s.MovePageAsPersona(ctx, ws, userID, p.ID, folder+"/"+linksTitle(*p.Path))
		return Result{Note: a}, err
	}
	if *merge == id {
		return Result{}, apperr.Invalid("merge", "cannot merge a page into itself")
	}
	ctx = pageview.Person(ctx, userID)
	var out Result
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		source, err := lockPersonaPage(ctx, q, ws, userID, id)
		if err != nil {
			return err
		}
		target, err := lockPersonaPage(ctx, q, ws, userID, *merge)
		if err != nil {
			return err
		}
		for _, p := range []store.Artifact{source, target} {
			if err := personaMay(p, "file", "filed"); err != nil {
				return err
			}
		}
		if source.Audience != target.Audience {
			return apperr.Invalid("merge", "pages must have the same audience; share explicitly before merging")
		}
		reviews, err := q.ListPageReviews(ctx, store.ListPageReviewsParams{ID: id, WorkspaceID: ws, Viewer: &userID})
		if err != nil {
			return err
		}
		if len(reviews) > 0 {
			return apperr.Conflict("the capture has changes waiting for review; the person reviews them before it's merged")
		}
		sourceBody, _, err := current(ctx, q, source)
		if err != nil {
			return err
		}
		targetBody, targetVersion, err := current(ctx, q, target)
		if err != nil {
			return err
		}
		out, err = s.write(ctx, q, ws, artifacts.Actor{Type: "agent", ID: agent.ID}, Save{Path: *target.Path, Body: targetBody + "\n\n" + sourceBody, BaseID: targetVersion, Note: "Merged capture: " + source.Title}, artifacts.Published)
		if err != nil {
			return err
		}
		return deletePage(ctx, q, source)
	})
	return out, err
}

func lockPersonaPage(ctx context.Context, q *store.Queries, ws, userID, id uuid.UUID) (store.Artifact, error) {
	a, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws, Viewer: &userID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.Type != "note") {
		return store.Artifact{}, apperr.ErrNotFound
	}
	return a, err
}
