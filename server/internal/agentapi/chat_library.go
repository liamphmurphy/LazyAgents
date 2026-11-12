package agentapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/linkpreview"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/transcript"
)

// The person's Library from a chat: find what they made with their personas
// (images, videos, diagrams, saved links) and save a link they ask to keep.
// Documents stay out: they're the person's own Docs, never agent context.

// libraryKinds are the kinds a persona may look through.
var libraryKinds = []string{"image", "video", "diagram", "link"}

const (
	searchLibraryDefault = 20
	searchLibraryMax     = 50
	searchLibraryWords   = 10
	savedLinkMaxTitle    = 300
	savedLinkMaxNote     = 300
	// A saved link with no page image: what the Library's own save stores then.
	savedLinkType = "text/uri-list"
)

type SearchLibraryIn struct {
	Query string `json:"q,omitempty" jsonschema:"Words that must all appear in the title, file name, note, site or address; empty lists the newest"`
	Kind  string `json:"kind,omitempty" jsonschema:"Only one kind: image, video, diagram or link"`
	Limit int    `json:"limit,omitempty" jsonschema:"Up to 50, default 20"`
}

// libraryItem is a Library entry as a persona sees it: what it is and where it
// came from, never its file.
type libraryItem struct {
	ID         uuid.UUID `json:"item_id"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	URL        string    `json:"url,omitempty"`
	Site       string    `json:"site,omitempty"`
	Note       string    `json:"note,omitempty"`
	LinkStatus string    `json:"link_status,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Size       string    `json:"size,omitempty"`
	Seconds    int32     `json:"seconds,omitempty"`
	Persona    string    `json:"persona,omitempty"`
	Chat       string    `json:"chat,omitempty"`
	ChatHref   string    `json:"chat_href,omitempty"`
	SavedAt    string    `json:"saved_at"`
}

func (it *libraryItem) media(w, h, ms *int32) {
	if w != nil && h != nil {
		it.Size = fmt.Sprintf("%d×%d", *w, *h)
	}
	if ms != nil {
		it.Seconds = *ms / 1000
	}
}

func searchLibrary(ctx context.Context, d Deps, c agents.Caller, in SearchLibraryIn) (any, error) {
	s, err := personChatScope(ctx, d, c, "search_library")
	if err != nil {
		return nil, err
	}
	arg := store.SearchLibraryItemsParams{WorkspaceID: c.Agent.WorkspaceID, UserID: s.UserID, Words: strings.Fields(in.Query), RowLimit: searchLibraryDefault}
	if in.Limit > searchLibraryMax {
		return nil, apperr.Invalid("limit", "at most %d", searchLibraryMax)
	}
	if in.Limit > 0 {
		arg.RowLimit = int32(in.Limit)
	}
	if len(arg.Words) > searchLibraryWords {
		return nil, apperr.Invalid("q", "use at most %d words", searchLibraryWords)
	}
	if k := strings.ToLower(strings.TrimSpace(in.Kind)); k != "" {
		if !slices.Contains(libraryKinds, k) {
			return nil, apperr.Invalid("kind", "image, video, diagram or link")
		}
		arg.Kind = &k
	}
	rows, err := d.Q.SearchLibraryItems(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := make([]libraryItem, len(rows))
	for i, r := range rows {
		out[i] = libraryItem{ID: r.ID, Kind: r.Kind, Title: r.Title, URL: r.Url, Site: r.Site, Persona: strOr(r.PersonaName), Chat: strOr(r.ChatTitle), SavedAt: r.CreatedAt.Format("2006-01-02")}
		if r.Kind == "link" {
			out[i].LinkStatus = r.LinkStatus
		}
		out[i].media(r.Width, r.Height, r.DurationMs)
	}
	return map[string]any{"items": out, "href": "/library"}, nil
}

type ReadLibraryItemIn struct {
	ItemID uuid.UUID `json:"item_id" jsonschema:"Item id from search_library"`
}

// readLibraryItem gives one entry in full: a link's note and saved summary,
// and the chat and persona it came from.
func readLibraryItem(ctx context.Context, d Deps, c agents.Caller, in ReadLibraryItemIn) (any, error) {
	s, err := personChatScope(ctx, d, c, "read_library_item")
	if err != nil {
		return nil, err
	}
	ws := c.Agent.WorkspaceID
	l, err := d.Q.GetLibraryItem(ctx, store.GetLibraryItemParams{ID: in.ItemID, WorkspaceID: ws, UserID: s.UserID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !slices.Contains(libraryKinds, l.Kind)) {
		return nil, apperr.Invalid("item_id", "no Library item %s", in.ItemID)
	} else if err != nil {
		return nil, err
	}
	out := libraryItem{ID: l.ID, Kind: l.Kind, Title: l.Title, URL: l.Url, Site: l.Site, Note: l.Note, SavedAt: l.CreatedAt.Format("2006-01-02")}
	if l.Kind == "link" {
		out.LinkStatus, out.Summary = l.LinkStatus, l.Summary
	}
	out.media(l.Width, l.Height, l.DurationMs)
	if l.PersonaID != nil {
		if p, err := d.Q.GetPersona(ctx, store.GetPersonaParams{ID: *l.PersonaID, WorkspaceID: ws, UserID: s.UserID}); err == nil {
			out.Persona = p.Name
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if l.ChatID != nil {
		if chat, err := d.Q.GetChatSession(ctx, store.GetChatSessionParams{ID: *l.ChatID, WorkspaceID: ws}); err == nil && chat.CreatedBy == s.UserID {
			out.Chat, out.ChatHref = chat.Title, "/chat/"+chat.ID.String()
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return map[string]any{"item": out, "href": "/library"}, nil
}

type SaveLinkIn struct {
	URL   string `json:"url" jsonschema:"The page or video to keep: an http(s) address the person gave or asked you to save"`
	Title string `json:"title,omitempty" jsonschema:"Its title, at most 300 characters; default its address"`
	Note  string `json:"note,omitempty" jsonschema:"Why it's worth reading, in a sentence, at most 300 characters"`
}

// saveLink adds a link to the person's reading list. It stores what the
// persona says about it and reads nothing from the page, so a model-chosen URL
// never makes Vesper fetch anything.
func saveLink(ctx context.Context, d Deps, c agents.Caller, in SaveLinkIn) (any, error) {
	s, err := personChatScope(ctx, d, c, "save_link")
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(in.URL)
	if !linkpreview.Valid(raw) {
		return nil, apperr.Invalid("url", "use an http(s) address with a host and no credentials, at most 2048 characters")
	}
	title, note := strings.TrimSpace(in.Title), strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(title) > savedLinkMaxTitle {
		return nil, apperr.Invalid("title", "at most %d characters", savedLinkMaxTitle)
	}
	if utf8.RuneCountInString(note) > savedLinkMaxNote {
		return nil, apperr.Invalid("note", "at most %d characters", savedLinkMaxNote)
	}
	ws := c.Agent.WorkspaceID
	if l, err := d.Q.GetLibraryLink(ctx, store.GetLibraryLinkParams{WorkspaceID: ws, UserID: s.UserID, Url: raw}); err == nil {
		return map[string]any{"item_id": l.ID, "title": l.Title, "state": "already_saved", "tell_the_person": "It was already in their Library."}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	u, _ := url.Parse(raw) // Valid parsed it already.
	site := strings.TrimPrefix(u.Hostname(), "www.")
	if title == "" {
		title = site + strings.TrimRight(u.Path, "/")
		if r := []rune(title); len(r) > savedLinkMaxTitle {
			title = string(r[:savedLinkMaxTitle])
		}
	}
	video, _ := transcript.VideoID(raw)
	chat := s.ChatID
	l, err := d.Q.CreateLibraryLink(ctx, store.CreateLibraryLinkParams{WorkspaceID: ws, UserID: s.UserID, Title: title, ContentType: savedLinkType, ChatID: &chat, PersonaID: s.PersonaID,
		Url: raw, Note: note, Site: site, VideoID: video})
	if err != nil {
		return nil, err
	}
	return map[string]any{"item_id": l.ID, "title": l.Title, "state": "saved", "href": "/library",
		"tell_the_person": "Saved to their reading list in the [Library](/library)."}, nil
}
