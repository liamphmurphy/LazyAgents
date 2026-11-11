// Package personas stores a person's Assistant personas (wiki: Architecture/ADR -
// Assistant personas). Personas are private: every call takes the Owner and
// every query filters on it, so another person's persona is ErrNotFound.
package personas

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/memory"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	MaxPerPerson    = 20
	MaxNameRunes    = 60
	MaxInstructions = 2000
	DefaultColor    = "#64748b"
	DefaultTone     = "neutral"
)

// Tones are the accepted tone values; each maps to one fixed prompt sentence.
var Tones = []string{"neutral", "friendly", "concise", "formal", "playful"}

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Owner is the person whose personas a call may see.
type Owner struct{ WorkspaceID, UserID uuid.UUID }

type Service struct{ Pool *pgxpool.Pool }

// NewInput creates a persona; nil fields take the defaults.
type NewInput struct {
	Name         string
	Tone         *string
	Color        *string
	Instructions *string
	IsDefault    bool
	// Capabilities and Repos are the structured settings that widen what the
	// persona's chats may do (runs.ChatSpec); nil takes the defaults (off, none).
	// Personas is whether it may propose new personas (each one approved by
	// the person); SVG whether its replies may show SVG images; Workflows
	// whether it may propose workflows and edits to them.
	Code, Images, Personas, SVG, Workflows, Connectors *bool
	Repos                                              *[]string
}

// Patch edits a persona at Revision; nil fields are unchanged.
type Patch struct {
	Revision                                           int32
	Name                                               *string
	Tone                                               *string
	Color                                              *string
	Instructions                                       *string
	Code, Images, Personas, SVG, Workflows, Connectors *bool
	// Repos replaces the whole list; existing chats keep the repo they chose.
	Repos *[]string
}

func normName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > MaxNameRunes {
		return "", apperr.Invalid("name", "must be 1–%d characters", MaxNameRunes)
	}
	return s, nil
}

func checkTone(s string) error {
	if !slices.Contains(Tones, s) {
		return apperr.Invalid("tone", "must be one of %s", strings.Join(Tones, ", "))
	}
	return nil
}

func normColor(s string) (string, error) {
	if !colorRe.MatchString(s) {
		return "", apperr.Invalid("color", "must be a #rrggbb colour")
	}
	return strings.ToLower(s), nil
}

func checkInstructions(s string) error {
	if utf8.RuneCountInString(s) > MaxInstructions {
		return apperr.Invalid("custom_instructions", "must be at most %d characters", MaxInstructions)
	}
	if kind := memory.DetectSecret(s); kind != "" {
		return apperr.Invalid("custom_instructions", "looks like it contains a %s; remove it", kind)
	}
	return nil
}

// tx runs fn in a transaction holding the owner's persona lock, so default
// swaps, the per-person cap and name checks never race.
func (s *Service) tx(ctx context.Context, o Owner, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := LockOwner(ctx, q, o); err != nil {
			return err
		}
		return fn(q)
	})
}

// nameTaken maps a lost race on personas_name (or any unique name clash) to a 422.
func nameTaken(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "personas_name" {
		return apperr.Invalid("name", "you already have a persona with this name")
	}
	return err
}

func checkNameFree(ctx context.Context, q *store.Queries, o Owner, name string, except *uuid.UUID) error {
	taken, err := q.PersonaNameTaken(ctx, store.PersonaNameTakenParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID, Name: name, ExceptID: except})
	if err != nil {
		return err
	}
	if taken {
		return apperr.Invalid("name", "you already have a persona with this name")
	}
	return nil
}

func get(ctx context.Context, q *store.Queries, o Owner, id uuid.UUID) (store.Persona, error) {
	p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperr.ErrNotFound
	}
	return p, err
}

// List returns the owner's personas: the default first, then most recently used, then by name.
func (s *Service) List(ctx context.Context, o Owner) ([]store.Persona, error) {
	return store.New(s.Pool).ListPersonas(ctx, store.ListPersonasParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID})
}

// Get returns one of the owner's personas, or ErrNotFound.
func (s *Service) Get(ctx context.Context, o Owner, id uuid.UUID) (store.Persona, error) {
	return get(ctx, store.New(s.Pool), o, id)
}

// Default returns the owner's default persona, or nil for the built-in Assistant.
func (s *Service) Default(ctx context.Context, o Owner) (*store.Persona, error) {
	p, err := store.New(s.Pool).DefaultPersona(ctx, store.DefaultPersonaParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Create adds a persona. With IsDefault it replaces the current default.
func (s *Service) Create(ctx context.Context, o Owner, in NewInput) (store.Persona, error) {
	var out store.Persona
	err := s.tx(ctx, o, func(q *store.Queries) error {
		var err error
		out, err = CreateLocked(ctx, q, o, in)
		return err
	})
	return out, err
}

// CreateLocked is Create inside the caller's transaction, which must already
// hold the owner's persona lock (LockPersonaOwner).
func CreateLocked(ctx context.Context, q *store.Queries, o Owner, in NewInput) (store.Persona, error) {
	var out store.Persona
	name, err := normName(in.Name)
	if err != nil {
		return out, err
	}
	tone, color, instr := DefaultTone, DefaultColor, ""
	if in.Tone != nil {
		tone = *in.Tone
	}
	if err := checkTone(tone); err != nil {
		return out, err
	}
	if in.Color != nil {
		if color, err = normColor(*in.Color); err != nil {
			return out, err
		}
	}
	if in.Instructions != nil {
		instr = *in.Instructions
	}
	if err := checkInstructions(instr); err != nil {
		return out, err
	}
	repos := []string{}
	if in.Repos != nil {
		if repos, err = NormalizeRepos(*in.Repos); err != nil {
			return out, err
		}
	}
	n, err := q.CountPersonas(ctx, store.CountPersonasParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID})
	if err != nil {
		return out, err
	}
	if n >= MaxPerPerson {
		return out, apperr.Invalid("personas", "you can have at most %d personas", MaxPerPerson)
	}
	if err := checkNameFree(ctx, q, o, name, nil); err != nil {
		return out, err
	}
	if in.IsDefault {
		if err := q.ClearDefaultPersona(ctx, store.ClearDefaultPersonaParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID, KeepID: uuid.Nil}); err != nil {
			return out, err
		}
	}
	out, err = q.CreatePersona(ctx, store.CreatePersonaParams{
		WorkspaceID: o.WorkspaceID, UserID: o.UserID, Name: name, Tone: tone, Color: color, CustomInstructions: instr, IsDefault: in.IsDefault,
		CanCode: in.Code != nil && *in.Code, CanGenerateImages: in.Images != nil && *in.Images, Repos: repos,
		CanCreatePersonas: in.Personas != nil && *in.Personas, CanCreateSvgs: in.SVG != nil && *in.SVG, CanCreateWorkflows: in.Workflows != nil && *in.Workflows,
		CanCreateConnectors: in.Connectors != nil && *in.Connectors,
	})
	if err == nil {
		for _, repo := range repos {
			if e := q.AddRepositorySource(ctx, store.AddRepositorySourceParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID, Kind: "repository", Name: repo}); e != nil {
				return out, e
			}
		}
	}
	return out, nameTaken(err)
}

// Draft is a persona another persona proposes: only the voice, never
// capabilities. CheckDraft normalizes it the way Create would.
type Draft struct {
	Name, Tone, Color, Instructions string
}

// CheckDraft validates and normalizes a proposed persona; empty tone and
// colour take the defaults.
func CheckDraft(d Draft) (Draft, error) {
	name, err := normName(d.Name)
	if err != nil {
		return d, err
	}
	d.Name = name
	if d.Tone == "" {
		d.Tone = DefaultTone
	}
	if err := checkTone(d.Tone); err != nil {
		return d, err
	}
	if d.Color == "" {
		d.Color = DefaultColor
	}
	if d.Color, err = normColor(d.Color); err != nil {
		return d, err
	}
	d.Instructions = strings.TrimSpace(d.Instructions)
	return d, checkInstructions(d.Instructions)
}

// LockOwner takes the owner's persona lock in the caller's transaction, for
// CreateLocked. Take it before any chat row lock: Delete holds it while it
// locks the persona's chats.
func LockOwner(ctx context.Context, q *store.Queries, o Owner) error {
	return q.LockPersonaOwner(ctx, store.LockPersonaOwnerParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID})
}

// Update edits a persona, checking the revision (stale is a 409). A change to
// the name, colour or capabilities re-renders the summary embedded in its chats, so those
// chats get chat.updated.
func (s *Service) Update(ctx context.Context, o Owner, id uuid.UUID, p Patch) (store.Persona, error) {
	var out store.Persona
	if p.Name != nil {
		name, err := normName(*p.Name)
		if err != nil {
			return out, err
		}
		p.Name = &name
	}
	if p.Tone != nil {
		if err := checkTone(*p.Tone); err != nil {
			return out, err
		}
	}
	if p.Color != nil {
		color, err := normColor(*p.Color)
		if err != nil {
			return out, err
		}
		p.Color = &color
	}
	if p.Instructions != nil {
		if err := checkInstructions(*p.Instructions); err != nil {
			return out, err
		}
	}
	var repos []string // nil leaves the list alone; an empty list clears it
	if p.Repos != nil {
		var err error
		if repos, err = NormalizeRepos(*p.Repos); err != nil {
			return out, err
		}
	}
	err := s.tx(ctx, o, func(q *store.Queries) error {
		cur, err := get(ctx, q, o, id)
		if err != nil {
			return err
		}
		if cur.Revision != p.Revision {
			return apperr.Conflict("the persona changed since you loaded it")
		}
		if p.Name != nil {
			if err := checkNameFree(ctx, q, o, *p.Name, &id); err != nil {
				return err
			}
		}
		out, err = q.UpdatePersona(ctx, store.UpdatePersonaParams{
			ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID, Revision: p.Revision,
			Name: p.Name, Tone: p.Tone, Color: p.Color, CustomInstructions: p.Instructions,
			CanCode: p.Code, CanGenerateImages: p.Images, CanCreatePersonas: p.Personas, CanCreateSvgs: p.SVG, CanCreateWorkflows: p.Workflows, CanCreateConnectors: p.Connectors, Repos: repos,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("the persona changed since you loaded it")
		}
		if err != nil {
			return nameTaken(err)
		}
		for _, repo := range repos {
			if e := q.AddRepositorySource(ctx, store.AddRepositorySourceParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID, Kind: "repository", Name: repo}); e != nil {
				return e
			}
		}
		if out.Name != cur.Name || out.Color != cur.Color || out.CanCode != cur.CanCode || out.CanGenerateImages != cur.CanGenerateImages || out.CanCreatePersonas != cur.CanCreatePersonas || out.CanCreateSvgs != cur.CanCreateSvgs || out.CanCreateWorkflows != cur.CanCreateWorkflows {
			return NotifyChats(ctx, q, id)
		}
		return nil
	})
	return out, err
}

// SetDefault makes a persona the owner's default (idempotent). The old default
// is cleared before the new one is set, in one transaction.
func (s *Service) SetDefault(ctx context.Context, o Owner, id uuid.UUID) (store.Persona, error) {
	var out store.Persona
	err := s.tx(ctx, o, func(q *store.Queries) error {
		var err error
		if out, err = get(ctx, q, o, id); err != nil || out.IsDefault {
			return err
		}
		if err := q.ClearDefaultPersona(ctx, store.ClearDefaultPersonaParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID, KeepID: id}); err != nil {
			return err
		}
		out, err = q.MarkDefaultPersona(ctx, store.MarkDefaultPersonaParams{ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID})
		return err
	})
	return out, err
}

// Delete removes a persona and returns the deleted row (its avatar blob is the
// caller's to remove). A non-nil revision must match (409 when stale). Its
// chats fall back to the built-in Assistant through ON DELETE SET NULL, which
// fires chat.updated. Deleting the default promotes the most recently used
// remaining persona; with none left, the built-in Assistant is the default.
func (s *Service) Delete(ctx context.Context, o Owner, id uuid.UUID, revision *int32) (store.Persona, error) {
	var out store.Persona
	err := s.tx(ctx, o, func(q *store.Queries) error {
		cur, err := get(ctx, q, o, id)
		if err != nil {
			return err
		}
		if revision != nil && *revision != cur.Revision {
			return apperr.Conflict("the persona changed since you loaded it")
		}
		if err := q.LockPersonaChats(ctx, id); err != nil {
			return err
		}
		if out, err = q.DeletePersona(ctx, store.DeletePersonaParams{ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID}); err != nil {
			return err
		}
		if out.IsDefault {
			return q.PromoteDefaultPersona(ctx, store.PromoteDefaultPersonaParams{WorkspaceID: o.WorkspaceID, UserID: o.UserID})
		}
		return nil
	})
	return out, err
}

// SetAvatar points the persona at an avatar already stored under key with the
// sniffed content type, or clears it when key is nil (a no-op without an
// avatar, so removing is idempotent). It returns the row and the key it
// replaced: that blob is the caller's to delete once this has committed. The
// owner lock serializes concurrent uploads, so each replaced key is returned
// to exactly one caller. Its chats get chat.updated, since their embedded
// summary carries the avatar URL.
func (s *Service) SetAvatar(ctx context.Context, o Owner, id uuid.UUID, key, contentType *string) (store.Persona, *string, error) {
	var out store.Persona
	var old *string
	if (key == nil) != (contentType == nil) {
		return out, nil, errors.New("personas: avatar key and content type go together")
	}
	err := s.tx(ctx, o, func(q *store.Queries) error {
		cur, err := get(ctx, q, o, id)
		if err != nil {
			return err
		}
		if key == nil && cur.AvatarKey == nil {
			out = cur
			return nil
		}
		out, err = q.SetPersonaAvatar(ctx, store.SetPersonaAvatarParams{
			ID: id, WorkspaceID: o.WorkspaceID, UserID: o.UserID, AvatarKey: key, AvatarContentType: contentType,
		})
		if err != nil {
			return err
		}
		old = cur.AvatarKey
		return NotifyChats(ctx, q, id)
	})
	if err != nil {
		return out, nil, err
	}
	return out, old, nil
}

// notifyBatch keeps each payload well under pg_notify's 8000 byte limit.
const notifyBatch = 50

// NotifyChats sends chat.updated for every chat using the persona, for changes
// that alter the embedded summary without writing chat_sessions (rename,
// colour, avatar). Run it inside the writing transaction: NOTIFY is delivered
// on commit.
func NotifyChats(ctx context.Context, q *store.Queries, persona uuid.UUID) error {
	ids, err := q.PersonaChatIDs(ctx, persona)
	if err != nil {
		return err
	}
	for len(ids) > 0 {
		n := min(len(ids), notifyBatch)
		batch := make([]string, n)
		for i, id := range ids[:n] {
			batch[i] = id.String()
		}
		if err := q.NotifyChatsUpdated(ctx, batch); err != nil {
			return err
		}
		ids = ids[n:]
	}
	return nil
}

// SeedDefault gives a new person the built-in "Assistant" default persona.
// It does nothing when they already have a default.
func SeedDefault(ctx context.Context, q *store.Queries, ws, user uuid.UUID) error {
	_, err := q.SeedDefaultPersona(ctx, store.SeedDefaultPersonaParams{WorkspaceID: ws, UserID: user})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
