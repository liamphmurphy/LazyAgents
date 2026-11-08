package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Server) ListChatCanvases(ctx context.Context, req api.ListChatCanvasesRequestObject) (api.ListChatCanvasesResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListChatCanvases(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	out := api.ChatCanvases{Canvases: make([]api.ChatCanvasSummary, 0, len(rows))}
	for _, r := range rows {
		out.Canvases = append(out.Canvases, api.ChatCanvasSummary{Id: r.ID, Kind: api.ChatCanvasSummaryKind(r.Kind), Title: r.Title, Seq: r.Seq, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return api.ListChatCanvases200JSONResponse(out), nil
}

func (s *Server) GetChatCanvas(ctx context.Context, req api.GetChatCanvasRequestObject) (api.GetChatCanvasResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	cv, err := s.Q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: req.CanvasId, SessionID: c.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cur, err := s.Q.LatestChatCanvasVersion(ctx, cv.ID)
	if err != nil {
		return nil, err
	}
	v := cur
	if req.Params.Seq != nil {
		v, err = s.Q.GetChatCanvasVersion(ctx, store.GetChatCanvasVersionParams{CanvasID: cv.ID, Seq: *req.Params.Seq})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
	}
	rows, err := s.Q.ListChatCanvasVersions(ctx, cv.ID)
	if err != nil {
		return nil, err
	}
	versions := make([]api.ChatCanvasVersion, 0, len(rows))
	for _, r := range rows {
		versions = append(versions, api.ChatCanvasVersion{Seq: r.Seq, Note: r.Note, AuthorType: api.ChatCanvasVersionAuthorType(r.AuthorType), CreatedAt: r.CreatedAt})
	}
	return api.GetChatCanvas200JSONResponse{Id: cv.ID, Kind: api.ChatCanvasKind(cv.Kind), Title: cv.Title, Seq: cur.Seq, CreatedAt: cv.CreatedAt, UpdatedAt: cv.UpdatedAt, ViewingSeq: v.Seq, Body: v.Body, Versions: versions, PromotedArtifactId: nullableOf(cv.PromotedArtifactID)}, nil
}

func nullableOf[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}

func (s *Server) canvases() *canvas.Service { return &canvas.Service{Pool: s.Pool} }

func myActor(ctx context.Context) canvas.Actor {
	u, _, _ := me(ctx)
	return canvas.Actor{Type: "user", ID: u.ID}
}

func toCanvasVersion(v store.ChatCanvasVersion) api.ChatCanvasVersion {
	return api.ChatCanvasVersion{Seq: v.Seq, Note: v.Note, AuthorType: api.ChatCanvasVersionAuthorType(v.AuthorType), CreatedAt: v.CreatedAt}
}

func toCanvasComment(r store.ChatCanvasComment) (api.ChatCanvasComment, error) {
	out := api.ChatCanvasComment{Id: r.ID, ParentId: nullableOf(r.ParentID), Seq: r.Seq, AuthorType: r.AuthorType, Body: r.Body, TargetLost: r.TargetLost, Status: r.Status, Suggestion: nullableOf(r.Suggestion), CreatedAt: r.CreatedAt}
	return out, json.Unmarshal(r.Anchor, &out.Anchor)
}

func (s *Server) SaveChatCanvas(ctx context.Context, req api.SaveChatCanvasRequestObject) (api.SaveChatCanvasResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	v, err := s.canvases().Save(ctx, c.ID, req.CanvasId, req.Body.BaseSeq, req.Body.Body, deref(req.Body.Note), myActor(ctx))
	if err != nil {
		return nil, err
	}
	return api.SaveChatCanvas201JSONResponse(toCanvasVersion(v)), nil
}

func (s *Server) RestoreChatCanvas(ctx context.Context, req api.RestoreChatCanvasRequestObject) (api.RestoreChatCanvasResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	v, err := s.canvases().Restore(ctx, c.ID, req.CanvasId, req.Body.Seq, myActor(ctx))
	if err != nil {
		return nil, err
	}
	return api.RestoreChatCanvas201JSONResponse(toCanvasVersion(v)), nil
}

// PromoteChatCanvas copies the current version into the project: mockups as
// mockup artifacts, documents as design documents.
func (s *Server) PromoteChatCanvas(ctx context.Context, req api.PromoteChatCanvasRequestObject) (api.PromoteChatCanvasResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	cv, err := s.Q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: req.CanvasId, SessionID: c.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if cv.PromotedArtifactID != nil {
		return api.PromoteChatCanvas200JSONResponse{ArtifactId: *cv.PromotedArtifactID}, nil
	}
	v, err := s.Q.LatestChatCanvasVersion(ctx, cv.ID)
	if err != nil {
		return nil, err
	}
	p, err := s.Q.GetProject(ctx, store.GetProjectParams{ID: c.ProjectID, WorkspaceID: c.WorkspaceID})
	if err != nil {
		return nil, err
	}
	u, _, ws := me(ctx)
	typ := "design"
	if cv.Kind == canvas.KindSVG {
		typ = "mockup"
	}
	a, err := s.Artifacts.Create(ctx, ws, artifacts.Actor{Type: "user", ID: u.ID}, artifacts.NewArtifact{ProjectKey: p.Key, Type: typ, Title: cv.Title, Content: artifacts.Content{BodyMD: &v.Body}})
	if err != nil {
		return nil, err
	}
	if err := s.Q.SetChatCanvasPromoted(ctx, store.SetChatCanvasPromotedParams{ID: cv.ID, PromotedArtifactID: &a.ID}); err != nil {
		return nil, err
	}
	return api.PromoteChatCanvas200JSONResponse{ArtifactId: a.ID}, nil
}

func (s *Server) ListChatCanvasComments(ctx context.Context, req api.ListChatCanvasCommentsRequestObject) (api.ListChatCanvasCommentsResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	cv, err := s.Q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: req.CanvasId, SessionID: c.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListChatCanvasComments(ctx, cv.ID)
	if err != nil {
		return nil, err
	}
	out := api.ChatCanvasComments{Comments: make([]api.ChatCanvasComment, 0, len(rows))}
	for _, r := range rows {
		x, err := toCanvasComment(r)
		if err != nil {
			return nil, err
		}
		out.Comments = append(out.Comments, x)
	}
	return api.ListChatCanvasComments200JSONResponse(out), nil
}

func (s *Server) CreateChatCanvasComment(ctx context.Context, req api.CreateChatCanvasCommentRequestObject) (api.CreateChatCanvasCommentResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	b := req.Body
	in := canvas.NewComment{ParentID: b.ParentId, Start: b.Start, End: b.End, VID: deref(b.Vid), Body: b.Body}
	if b.Seq != nil {
		in.Seq = *b.Seq
	}
	if b.Point != nil {
		in.Point = &canvas.Point{X: b.Point.X, Y: b.Point.Y}
	}
	r, err := s.canvases().AddComment(ctx, c.ID, req.CanvasId, in, myActor(ctx))
	if err != nil {
		return nil, err
	}
	out, err := toCanvasComment(r)
	return api.CreateChatCanvasComment201JSONResponse(out), err
}

func (s *Server) UpdateChatCanvasComment(ctx context.Context, req api.UpdateChatCanvasCommentRequestObject) (api.UpdateChatCanvasCommentResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	r, err := s.canvases().SetStatus(ctx, c.ID, req.CanvasId, req.CommentId, req.Body.Status)
	if err != nil {
		return nil, err
	}
	out, err := toCanvasComment(r)
	return api.UpdateChatCanvasComment200JSONResponse(out), err
}

func (s *Server) DeleteChatCanvasComment(ctx context.Context, req api.DeleteChatCanvasCommentRequestObject) (api.DeleteChatCanvasCommentResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if err := s.canvases().DeleteDraft(ctx, c.ID, req.CanvasId, req.CommentId); err != nil {
		return nil, err
	}
	return api.DeleteChatCanvasComment204Response{}, nil
}

func (s *Server) ApplyChatCanvasComment(ctx context.Context, req api.ApplyChatCanvasCommentRequestObject) (api.ApplyChatCanvasCommentResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	v, err := s.canvases().ApplySuggestion(ctx, c.ID, req.CanvasId, req.CommentId, myActor(ctx))
	if err != nil {
		return nil, err
	}
	return api.ApplyChatCanvasComment201JSONResponse(toCanvasVersion(v)), nil
}

// SendChatCanvasComments opens the drafts and posts one message carrying the
// canvas as context. If the message can't be posted the drafts stay drafts.
func (s *Server) SendChatCanvasComments(ctx context.Context, req api.SendChatCanvasCommentsRequestObject) (api.SendChatCanvasCommentsResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	cv, err := s.Q.GetChatCanvas(ctx, store.GetChatCanvasParams{ID: req.CanvasId, SessionID: c.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	opened, err := s.canvases().Open(ctx, c.ID, cv.ID, req.Body.CommentIds)
	if err != nil {
		return nil, err
	}
	body := strings.TrimSpace(deref(req.Body.Message))
	if body == "" {
		n := len(req.Body.CommentIds)
		noun := "comments"
		if n == 1 {
			noun = "comment"
		}
		// The title is the agent's text: it mustn't summon a persona.
		body = fmt.Sprintf("I left %d %s on “%s”.", n, noun, strings.ReplaceAll(cv.Title, "@", ""))
	}
	u, _, ws := me(ctx)
	id := cv.ID
	msg, err := s.Runs.PostChatMessage(ctx, ws, u.ID, c.ID, runs.ChatMessageInput{Body: body, ContextRefs: []runs.ChatContextRef{{Kind: "canvas", ID: &id}}})
	if err != nil {
		if rerr := s.Q.ReturnChatCanvasCommentsToDraft(context.WithoutCancel(ctx), opened); rerr != nil {
			return nil, errors.Join(err, rerr)
		}
		return nil, err
	}
	if err := s.Q.SetChatCanvasCommentsSent(ctx, store.SetChatCanvasCommentsSentParams{MessageID: &msg.ID, Ids: opened}); err != nil {
		return nil, err
	}
	return api.SendChatCanvasComments204Response{}, nil
}
