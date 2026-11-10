// Package docs is the Docs app: documents the person writes with the
// assistant. A document is a text canvas; one started from the app lives in
// its own chat in the person's Docs space, which keeps those chats out of the
// chat list and the Spaces menu.
package docs

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// AppName marks the Docs app's space in chat_spaces.app.
const AppName = "docs"

// SpaceInstructions reach every chat the Docs app starts.
const SpaceInstructions = `This chat belongs to one document in the person's Docs app: the text canvas in this chat. They see it full page and talk to you from a drawer beside it. Work on that document: canvas_read it, then make changes with canvas_patch rather than pasting text into your reply or creating another canvas, unless they ask for a new document. Keep replies short, saying what you changed.`

// UntitledTitle names a document started without a title.
const UntitledTitle = "Untitled document"

type Service struct {
	Pool *pgxpool.Pool
	Runs *runs.Service
}

// EnsureSpace is the person's Docs space, made the first time it's needed.
func (s *Service) EnsureSpace(ctx context.Context, ws, user uuid.UUID) (store.ChatSpace, error) {
	q := store.New(s.Pool)
	sp, err := q.GetAppSpace(ctx, store.GetAppSpaceParams{WorkspaceID: ws, UserID: user, AppName: AppName})
	if !errors.Is(err, pgx.ErrNoRows) {
		return sp, err
	}
	return q.CreateAppSpace(ctx, store.CreateAppSpaceParams{WorkspaceID: ws, UserID: user, Name: "Docs", Color: "blue",
		Description: "Documents you write with the assistant.", Instructions: SpaceInstructions, AppName: AppName})
}

// Create starts a document: a chat in the Docs space holding a new text
// canvas whose first version is the title as a heading.
func (s *Service) Create(ctx context.Context, ws, user uuid.UUID, title string) (store.ChatCanvas, store.ChatSession, error) {
	var cv store.ChatCanvas
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		title = UntitledTitle
	}
	sp, err := s.EnsureSpace(ctx, ws, user)
	if err != nil {
		return cv, store.ChatSession{}, err
	}
	chat, err := s.Runs.StartChat(ctx, ws, user, runs.ChatInput{Title: title, SpaceID: &sp.ID})
	if err != nil {
		return cv, chat, err
	}
	cv, _, err = (&canvas.Service{Pool: s.Pool}).Create(ctx, chat.ID, canvas.New{Kind: canvas.KindText, Title: title, Body: "# " + title + "\n"}, canvas.Actor{Type: "user", ID: user})
	return cv, chat, err
}
