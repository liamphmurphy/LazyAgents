// Package canvas holds the documents and SVG mockups that sit beside a chat.
package canvas

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/deck"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	KindText = "text"
	KindSVG  = "svg"
	KindDeck = "deck"
	// maxBody bounds one version, in bytes.
	maxBody = 512 * 1024
	maxOps  = 100
)

type Service struct{ Pool *pgxpool.Pool }

// Actor is who wrote a version.
type Actor struct {
	Type string
	ID   uuid.UUID
}

type New struct {
	Kind, Title, Body, Note string
}

// Op is one edit. A text canvas uses Find and Replace; an SVG canvas uses Op,
// VID and whichever of the other fields that op needs; a deck uses Op, ID,
// After, Slide and Value.
type Op struct {
	Find     string            `json:"find,omitempty" jsonschema:"Text canvas: the exact text to change; it must occur exactly once"`
	Replace  string            `json:"replace,omitempty" jsonschema:"Text canvas: what to put in its place; omit to delete it"`
	Op       string            `json:"op,omitempty" jsonschema:"SVG canvas: set_attrs, set_text, replace, insert or remove. Deck: set_theme, insert_slide, set_slide, move_slide, remove_slide, insert_element, set_element or remove_element"`
	VID      string            `json:"vid,omitempty" jsonschema:"SVG canvas: the data-vid of the element to act on. Omit only to append to the root"`
	Attrs    map[string]string `json:"attrs,omitempty" jsonschema:"set_attrs: attributes to set; an empty value removes one"`
	Text     string            `json:"text,omitempty" jsonschema:"set_text: the element's new text"`
	Markup   string            `json:"markup,omitempty" jsonschema:"replace and insert: exactly one SVG element"`
	Position string            `json:"position,omitempty" jsonschema:"insert: before or after the vid element, or append inside it"`
	ID       string            `json:"id,omitempty" jsonschema:"Deck: the slide or element id the op acts on"`
	After    string            `json:"after,omitempty" jsonschema:"Deck insert_slide, move_slide, insert_element: the id to place it after; omit to put a slide first or an element on top"`
	Slide    string            `json:"slide,omitempty" jsonschema:"Deck insert_element: the slide id to add the element to"`
	Value    map[string]any    `json:"value,omitempty" jsonschema:"Deck: the slide (insert_slide), element (insert_element), fields to set (set_slide, set_element, set_theme) or {preset: light|dark|paper|contrast} for set_theme. Omit ids on new things; the server assigns them"`
}

func normalize(kind, body string) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", apperr.Invalid("body", "provide some content")
	}
	if len(body) > maxBody || !utf8.ValidString(body) {
		return "", apperr.Invalid("body", "must be valid UTF-8 and at most %d KB", maxBody/1024)
	}
	if kind == KindSVG {
		return Stamp(body)
	}
	if kind == KindDeck {
		return deck.Normalize(body)
	}
	return body, nil
}

func applyText(body string, i int, op Op) (string, error) {
	if op.Find == "" {
		return "", apperr.Invalid("ops", "op %d: find is required on a text canvas", i)
	}
	if n := strings.Count(body, op.Find); n != 1 {
		return "", apperr.Invalid("ops", "op %d: find matched %d times; it must match exactly once, so include more surrounding text", i, n)
	}
	return strings.Replace(body, op.Find, op.Replace, 1), nil
}

// Apply runs ops in order on a body and returns the normalized result.
func Apply(kind, body string, ops []Op) (string, error) {
	if len(ops) == 0 || len(ops) > maxOps {
		return "", apperr.Invalid("ops", "provide 1 to %d ops", maxOps)
	}
	if kind == KindDeck {
		dops := make([]deck.Op, len(ops))
		for i, op := range ops {
			dops[i] = deck.Op{Op: op.Op, ID: op.ID, After: op.After, Slide: op.Slide, Value: op.Value}
		}
		return deck.Apply(body, dops)
	}
	for i, op := range ops {
		var err error
		if kind == KindSVG {
			var all []*element
			if all, err = scan(body); err == nil {
				body, err = applySVGOp(body, all, op)
			}
			if err != nil {
				return "", fmt.Errorf("op %d: %w", i, err)
			}
			continue
		}
		if body, err = applyText(body, i, op); err != nil {
			return "", err
		}
	}
	return normalize(kind, body)
}

// Create makes a canvas and its first version.
func (s *Service) Create(ctx context.Context, chat uuid.UUID, in New, by Actor) (store.ChatCanvas, store.ChatCanvasVersion, error) {
	var c store.ChatCanvas
	var v store.ChatCanvasVersion
	title := strings.Join(strings.Fields(in.Title), " ")
	if n := utf8.RuneCountInString(title); n < 1 || n > 200 {
		return c, v, apperr.Invalid("title", "provide 1–200 characters")
	}
	if in.Kind != KindText && in.Kind != KindSVG && in.Kind != KindDeck {
		return c, v, apperr.Invalid("kind", "use text, svg or deck")
	}
	body, err := normalize(in.Kind, in.Body)
	if err != nil {
		return c, v, err
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if c, err = q.CreateChatCanvas(ctx, store.CreateChatCanvasParams{SessionID: chat, Kind: in.Kind, Title: title}); err != nil {
			return err
		}
		v, err = q.CreateChatCanvasVersion(ctx, store.CreateChatCanvasVersionParams{CanvasID: c.ID, Seq: 1, Body: body, Note: in.Note, AuthorType: by.Type, AuthorID: by.ID})
		return err
	})
	return c, v, err
}

// Patch applies ops on top of version base, which must still be current: an
// edit made against text someone has since changed is refused, not merged.
func (s *Service) Patch(ctx context.Context, chat, id uuid.UUID, base int32, ops []Op, note string, by Actor) (store.ChatCanvasVersion, error) {
	var v store.ChatCanvasVersion
	err := s.locked(ctx, chat, id, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		if base != cur.Seq {
			return apperr.Conflict("this canvas is at version %d, not %d; read it again and redo your edit", cur.Seq, base)
		}
		body, err := Apply(c.Kind, cur.Body, ops)
		if err != nil {
			return err
		}
		if body == cur.Body {
			return apperr.Invalid("ops", "these ops change nothing")
		}
		v, err = writeVersion(ctx, q, c, cur, body, note, by)
		return err
	})
	return v, err
}
