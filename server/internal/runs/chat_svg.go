package runs

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// svgFenceRe matches the opening of an ```svg-image fence wherever Markdown
// could start one, including inside a list or blockquote.
var svgFenceRe = regexp.MustCompile("(?i)(`{3,}|~{3,})([ \t]*)svg-image\\b")

// SpeakerCanCreateSVGs reports whether the persona answering the chat's turn
// (its speaker, else its host) may show SVG images. A deleted persona and the
// built-in Assistant can't.
func SpeakerCanCreateSVGs(ctx context.Context, q *store.Queries, chat store.ChatSession) (bool, error) {
	id := ChatSpeakerID(chat)
	if id == nil {
		return false, nil
	}
	p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *id, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return p.CanCreateSvgs, nil
}

// SpeakerCanCreateWorkflows reports whether the persona answering the chat's
// turn may propose workflows. A deleted persona and the built-in Assistant can't.
func SpeakerCanCreateWorkflows(ctx context.Context, q *store.Queries, chat store.ChatSession) (bool, error) {
	id := ChatSpeakerID(chat)
	if id == nil {
		return false, nil
	}
	p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *id, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return p.CanCreateWorkflows, nil
}

// GateSVGFences turns the svg-image fences in a reply into ordinary svg code
// blocks when its speaker may not show SVG images, so the capability doesn't
// rest on the prompt. It runs when the reply is stored: replies already in a
// chat keep their images if the capability is later turned off.
func GateSVGFences(ctx context.Context, q *store.Queries, chat store.ChatSession, body string) (string, error) {
	if !svgFenceRe.MatchString(body) {
		return body, nil
	}
	can, err := SpeakerCanCreateSVGs(ctx, q, chat)
	if err != nil || can {
		return body, err
	}
	return svgFenceRe.ReplaceAllString(body, "${1}${2}svg"), nil
}
