package wiki

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// MoveCanvasToPage is "Save to Knowledge" for a text canvas in an ordinary
// chat: the canvas becomes a private page with every version, its author and
// its comments, the chat stays attached to the page, and the canvas is deleted
// so the two can't drift apart the way "Promote" copies did. A Doc the
// migration already moved returns its existing page instead of a second one.
func (s *Service) MoveCanvasToPage(ctx context.Context, ws, userID, chatID, canvasID uuid.UUID, folder string) (Result, error) {
	ctx = pageview.Person(ctx, userID)
	var res Result
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		chat, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chatID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && chat.CreatedBy != userID) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if moved, err := q.GetDocPageRedirect(ctx, store.GetDocPageRedirectParams{CanvasID: canvasID, WorkspaceID: ws, Viewer: &userID}); err == nil {
			res = Result{Note: moved, State: artifacts.Published}
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		cv, err := q.LockChatCanvas(ctx, store.LockChatCanvasParams{ID: canvasID, SessionID: chat.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if cv.Kind != "text" {
			return apperr.Invalid("canvas", "only text canvases can be saved to Knowledge")
		}
		versions, err := q.ListChatCanvasVersions(ctx, cv.ID)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			return apperr.Invalid("canvas", "has no content to save")
		}
		// Oldest first, so the page's history reads in the canvas's order.
		for i := len(versions) - 1; i >= 0; i-- {
			v, err := q.GetChatCanvasVersion(ctx, store.GetChatCanvasVersionParams{CanvasID: cv.ID, Seq: versions[i].Seq})
			if err != nil {
				return err
			}
			save := Save{Body: v.Body, Note: v.Note, Audience: "private"}
			if res.Note.ID == uuid.Nil {
				title := cv.Title
				save.Title, save.AutoPath, save.Folder, save.Mode = &title, true, folder, ModeCreate
			} else {
				id := res.Note.ID
				save.PageID, save.Path, save.Mode = &id, deref(res.Note.Path), ModeReplace
			}
			res, err = s.write(ctx, q, ws, artifacts.Actor{Type: v.AuthorType, ID: v.AuthorID}, save, artifacts.Published)
			if err != nil {
				return err
			}
		}
		if err := q.SetPageChat(ctx, store.SetPageChatParams{ID: res.Note.ID, WorkspaceID: ws, Viewer: &userID, ChatID: &chat.ID}); err != nil {
			return err
		}
		if err := q.CopyCanvasCommentsToPage(ctx, store.CopyCanvasCommentsToPageParams{ArtifactID: res.Note.ID, WorkspaceID: ws, CanvasID: cv.ID, Viewer: &userID}); err != nil {
			return err
		}
		return q.DeleteChatCanvas(ctx, store.DeleteChatCanvasParams{ID: cv.ID, SessionID: chat.ID})
	})
	return res, err
}
