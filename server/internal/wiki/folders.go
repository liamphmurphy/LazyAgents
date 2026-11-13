package wiki

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Folders aren't rows: a folder is the shared prefix of its pages' paths, as
// in an Obsidian vault. Moving or deleting one acts on every page under it in
// one transaction, so a clash part way through leaves nothing half-moved.

// DeletePageAsUser deletes a page the person can see, with its history. Links
// into it go back to unresolved, so [[Name]] stays in the text and reattaches
// if a page by that name is made again.
func (s *Service) DeletePageAsUser(ctx context.Context, ws, userID, id uuid.UUID) error {
	ctx = pageview.Person(ctx, userID)
	return s.tx(ctx, ws, func(q *store.Queries) error {
		a, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws, Viewer: &userID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.Type != "note") {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		return deletePage(ctx, q, a)
	})
}

func deletePage(ctx context.Context, q *store.Queries, a store.Artifact) error {
	if err := q.UnresolveLinksTo(ctx, store.UnresolveLinksToParams{WorkspaceID: a.WorkspaceID, TargetID: &a.ID}); err != nil {
		return err
	}
	if err := q.DeleteLinksFromSource(ctx, store.DeleteLinksFromSourceParams{SourceKind: "artifact", SourceID: a.ID}); err != nil {
		return err
	}
	return q.DeleteArtifact(ctx, store.DeleteArtifactParams{ID: a.ID, WorkspaceID: a.WorkspaceID})
}

// FolderScope picks the explorer view a folder belongs to: agent knowledge,
// or the person's own pages. A folder action never reaches into the other.
type FolderScope struct {
	Folder string
	Agent  bool
}

func (f FolderScope) clean() (string, error) {
	p := strings.Trim(strings.TrimSpace(f.Folder), "/")
	if p == "" {
		return "", apperr.Invalid("folder", "is required")
	}
	return p, nil
}

// MoveFolder moves every page under from to the same place under to: renaming
// a folder is moving it beside itself. Each page keeps its old path as an
// alias, as a single rename does. It returns the pages it moved.
func (s *Service) MoveFolder(ctx context.Context, ws, userID uuid.UUID, from FolderScope, to string) ([]uuid.UUID, error) {
	src, err := from.clean()
	if err != nil {
		return nil, err
	}
	dst := strings.Trim(strings.TrimSpace(to), "/")
	if dst == "" {
		return nil, apperr.Invalid("to", "is required")
	}
	if dst == src {
		return []uuid.UUID{}, nil
	}
	if strings.HasPrefix(dst+"/", src+"/") {
		return nil, apperr.Invalid("to", "can't move a folder into itself")
	}
	ctx = pageview.Person(ctx, userID)
	moved := []uuid.UUID{}
	err = s.tx(ctx, ws, func(q *store.Queries) error {
		pages, err := q.LockNotesInFolder(ctx, store.LockNotesInFolderParams{WorkspaceID: ws, Folder: src, Viewer: &userID, Agent: from.Agent})
		if err != nil {
			return err
		}
		if len(pages) == 0 {
			return apperr.ErrNotFound
		}
		for _, p := range pages {
			if _, err := renameNote(ctx, q, ws, p.ID, dst+strings.TrimPrefix(*p.Path, src)); err != nil {
				return err
			}
			moved = append(moved, p.ID)
		}
		return nil
	})
	return moved, err
}

// DeleteFolder deletes every page under a folder, as DeletePageAsUser does
// one. It returns the pages it deleted.
func (s *Service) DeleteFolder(ctx context.Context, ws, userID uuid.UUID, folder FolderScope) ([]uuid.UUID, error) {
	src, err := folder.clean()
	if err != nil {
		return nil, err
	}
	ctx = pageview.Person(ctx, userID)
	deleted := []uuid.UUID{}
	err = s.tx(ctx, ws, func(q *store.Queries) error {
		pages, err := q.LockNotesInFolder(ctx, store.LockNotesInFolderParams{WorkspaceID: ws, Folder: src, Viewer: &userID, Agent: folder.Agent})
		if err != nil {
			return err
		}
		if len(pages) == 0 {
			return apperr.ErrNotFound
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
