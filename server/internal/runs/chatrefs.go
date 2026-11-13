package runs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// ChatContextRef is something a person attached to a chat message. It mirrors
// the API's ChatContextRef and is stored as-is in chat_messages.context_refs.
type ChatContextRef struct {
	Kind   string     `json:"kind"` // artifact, section, ticket, page or canvas
	ID     *uuid.UUID `json:"id,omitempty"`
	Key    string     `json:"key,omitempty"` // ticket key; for a page, optionally the wiki path
	Anchor string     `json:"anchor,omitempty"`
	// Selection is what the person had selected in a design (kind canvas only).
	Selection *canvas.Selection `json:"selection,omitempty"`
}

// MaxChatContextRefs matches the contract's maxItems for context_refs.
const MaxChatContextRefs = 20

func (r ChatContextRef) String() string {
	switch {
	case r.ID != nil && r.Anchor != "":
		return fmt.Sprintf("%s %s#%s", r.Kind, r.ID, r.Anchor)
	case r.ID != nil:
		return fmt.Sprintf("%s %s", r.Kind, r.ID)
	}
	return r.Kind + " " + r.Key
}

// ChatRefDoc is a resolved reference: the text a turn is given for it.
type ChatRefDoc struct {
	Ref        ChatContextRef
	Title      string
	Body       string
	ArtifactID *uuid.UUID // set for artifact-backed refs
}

// ResolveChatRefs resolves every ref inside the workspace and returns the
// first failure as a 422 naming it (field context_refs). The chat's project
// doesn't narrow it: the person can open any project's documents and tickets,
// and attaching one is an explicit ask to read it. Nothing is returned for a
// partly resolvable list, so callers queue nothing.
func ResolveChatRefs(ctx context.Context, q *store.Queries, ws, chat uuid.UUID, refs []ChatContextRef) ([]ChatRefDoc, error) {
	ctx = pageview.Run(ctx)
	if chat != uuid.Nil {
		session, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chat, WorkspaceID: ws})
		if err != nil {
			return nil, err
		}
		ctx = pageview.Person(ctx, session.CreatedBy)
	}
	if len(refs) > MaxChatContextRefs {
		return nil, apperr.Invalid("context_refs", "attach at most %d references", MaxChatContextRefs)
	}
	out := make([]ChatRefDoc, 0, len(refs))
	for _, r := range refs {
		d, err := resolveChatRef(ctx, q, ws, chat, r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func resolveChatRef(ctx context.Context, q *store.Queries, ws, chat uuid.UUID, r ChatContextRef) (ChatRefDoc, error) {
	r.Key, r.Anchor = strings.TrimSpace(r.Key), strings.TrimSpace(r.Anchor)
	unknown := apperr.Invalid("context_refs", "%s was not found", r)
	bad := func(why string) error { return apperr.Invalid("context_refs", "%s: %s", r, why) }
	d := ChatRefDoc{Ref: r}
	switch r.Kind {
	case "canvas":
		if r.ID == nil || r.Key != "" || r.Anchor != "" {
			return d, bad("a canvas ref needs an id and nothing else")
		}
		var title, body string
		var err error
		if r.Selection != nil {
			if err = r.Selection.Check(); err != nil {
				return d, err
			}
			title, body, err = canvas.SelectionDoc(ctx, q, chat, *r.ID, *r.Selection)
		} else {
			title, body, err = canvas.ContextDoc(ctx, q, chat, *r.ID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return d, unknown
		}
		if err != nil {
			return d, err
		}
		d.Title, d.Body = title, body
		return d, nil
	case "ticket":
		if r.Selection != nil {
			return d, bad("only a canvas ref has a selection")
		}
		if r.Key == "" || r.ID != nil {
			return d, bad("a ticket ref needs a key and no id")
		}
		t, err := q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: ws, Key: r.Key})
		if errors.Is(err, pgx.ErrNoRows) {
			return d, unknown
		}
		if err != nil {
			return d, err
		}
		d.Title = t.Key + " " + t.Title
		d.Body = fmt.Sprintf("# %s %s\n\nStatus: %s\nPriority: %s\n\n%s\n", t.Key, t.Title, t.Status, t.Priority, t.DescriptionMd)
		return d, nil
	case "artifact", "section", "page":
		if r.Selection != nil {
			return d, bad("only a canvas ref has a selection")
		}
		var a store.Artifact
		var err error
		switch {
		case r.ID != nil && r.Key == "":
			a, err = q.GetArtifact(ctx, store.GetArtifactParams{ID: *r.ID, WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
		case r.Kind == "page" && r.ID == nil && r.Key != "":
			a, err = q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: r.Key, Viewer: pageview.From(ctx).UserID})
		default:
			return d, bad("give exactly one of id or key (key only for a page's wiki path)")
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return d, unknown
		}
		if err != nil {
			return d, err
		}
		if r.Kind == "page" && a.Type != "note" {
			return d, bad("not a wiki page")
		}
		if (r.Kind == "section") != (r.Anchor != "") {
			return d, bad("a section ref needs an anchor, other refs must not have one")
		}
		if a.CurrentVersionID == nil {
			return d, bad("has no published version")
		}
		v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: ws, Viewer: pageview.From(ctx).UserID})
		if err != nil {
			return d, err
		}
		if v.BodyMd == nil {
			return d, bad("not a text document")
		}
		d.Title, d.Body, d.ArtifactID = a.Title, *v.BodyMd, &a.ID
		if r.Kind == "section" {
			sec, err := q.GetSectionByAnchor(ctx, store.GetSectionByAnchorParams{VersionID: v.ID, Anchor: r.Anchor})
			if errors.Is(err, pgx.ErrNoRows) {
				return d, unknown
			}
			if err != nil {
				return d, err
			}
			d.Title += " § " + sec.Heading
			d.Body = "<!-- the person pointed at the section \"" + sec.Heading + "\" -->\n" + d.Body
		}
		return d, nil
	}
	return d, bad("kind must be artifact, section, ticket, page or canvas")
}
