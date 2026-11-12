// Package designs is the Designs app (docs/design/ADR-designs.md): designs the
// person builds with a persona. A design is a canvas of kind "design" in its
// own chat in the person's Designs space, like a Slides deck. Its images are
// design_assets rows over content-addressed blobs (swept with the decks'),
// its switches are design_settings, and a persona's patches are held as
// design_proposals until the person accepts or rejects each op.
package designs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/design"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/slides"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// AppName marks the Designs app's space in chat_spaces.app.
const AppName = "designs"

// SpaceInstructions reach every chat the Designs app starts. The design_*
// tools and their guidance arrive with ticket 4 (ADR §3.9).
const SpaceInstructions = `This chat belongs to one design in the person's Designs app: the design canvas in this chat. They see it in an editor and talk to you from a panel beside it. Your changes arrive as suggestions the person accepts or rejects one by one, unless they let you edit directly. Give every change a specific why, use the design's tokens rather than hex, and never paste the design's JSON into your reply. Keep replies short, saying what you changed.`

// UntitledTitle names a design started without a title.
const UntitledTitle = "Untitled design"

// MaxAssetEdge caps a design image's longest side (ADR §3.6).
const MaxAssetEdge = 4096

const (
	maxSummary = 500
	maxReason  = 500
)

type Service struct {
	Pool *pgxpool.Pool
	Runs *runs.Service
}

func (s *Service) canvases() *canvas.Service { return &canvas.Service{Pool: s.Pool} }

// EnsureSpace is the person's Designs space, made the first time it's needed.
func (s *Service) EnsureSpace(ctx context.Context, ws, user uuid.UUID) (store.ChatSpace, error) {
	q := store.New(s.Pool)
	sp, err := q.GetAppSpace(ctx, store.GetAppSpaceParams{WorkspaceID: ws, UserID: user, AppName: AppName})
	if !errors.Is(err, pgx.ErrNoRows) {
		return sp, err
	}
	return q.CreateAppSpace(ctx, store.CreateAppSpaceParams{WorkspaceID: ws, UserID: user, Name: "Designs", Color: "teal",
		Description: "Flows, diagrams and wireframes you design with the assistant.", Instructions: SpaceInstructions, AppName: AppName})
}

// CleanTitle collapses whitespace and checks the length.
func CleanTitle(title string) (string, error) {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		title = UntitledTitle
	}
	if utf8.RuneCountInString(title) > 200 {
		return "", apperr.Invalid("title", "must be at most 200 characters")
	}
	return title, nil
}

// start makes a chat in the Designs space holding a design canvas with body,
// and records the project the chat ended up in.
func (s *Service) start(ctx context.Context, ws, user uuid.UUID, title, body, note, project string) (store.ChatCanvas, store.ChatSession, error) {
	sp, err := s.EnsureSpace(ctx, ws, user)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	chat, err := s.Runs.StartChat(ctx, ws, user, runs.ChatInput{Title: title, SpaceID: &sp.ID, Project: project})
	if err != nil {
		return store.ChatCanvas{}, chat, err
	}
	cv, _, err := s.canvases().Create(ctx, chat.ID, canvas.New{Kind: canvas.KindDesign, Title: title, Body: body, Note: note}, canvas.Actor{Type: "user", ID: user})
	if err != nil {
		return cv, chat, err
	}
	if chat.ProjectID != nil {
		err = store.New(s.Pool).UpsertDesignSettings(ctx, store.UpsertDesignSettingsParams{CanvasID: cv.ID, ProjectID: chat.ProjectID})
	}
	return cv, chat, err
}

// Create starts a design from a starter (design.Starters; "" is blank).
// project is a project key, or empty.
func (s *Service) Create(ctx context.Context, ws, user uuid.UUID, title, starter, project string) (store.ChatCanvas, store.ChatSession, error) {
	title, err := CleanTitle(title)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	body, err := design.Starter(starter)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	note := ""
	if starter != "" && starter != "blank" {
		note = "Started from the " + starter + " starter"
	}
	return s.start(ctx, ws, user, title, body, note, project)
}

// Duplicate copies a design's current version, with fresh ids, its images and
// its project into a new design and chat. Proposals and switches stay behind.
func (s *Service) Duplicate(ctx context.Context, ws, user uuid.UUID, src store.GetMyCanvasRow) (store.ChatCanvas, store.ChatSession, error) {
	q := store.New(s.Pool)
	cur, err := q.LatestChatCanvasVersion(ctx, src.ID)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	body, err := design.Remint(cur.Body)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	set, err := q.GetDesignSettings(ctx, src.ID)
	if err != nil {
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	project := ""
	if set.ProjectID != nil {
		p, err := q.GetProject(ctx, store.GetProjectParams{ID: *set.ProjectID, WorkspaceID: ws})
		if err == nil {
			project = p.Key
		}
	}
	title := "Copy of " + src.Title
	if r := []rune(title); len(r) > 200 {
		title = string(r[:200])
	}
	cv, chat, err := s.start(ctx, ws, user, title, body, fmt.Sprintf("Copied from %s", src.Title), project)
	if err != nil {
		return cv, chat, err
	}
	if err := q.CopyDesignAssets(ctx, store.CopyDesignAssetsParams{ToCanvas: cv.ID, FromCanvas: src.ID}); err != nil {
		// Don't leave a copy whose images are missing.
		_ = s.Runs.DeleteChat(context.WithoutCancel(ctx), ws, user, chat.ID)
		return store.ChatCanvas{}, store.ChatSession{}, err
	}
	return cv, chat, nil
}

// Rename sets a design's title.
func (s *Service) Rename(ctx context.Context, cv store.GetMyCanvasRow, title string) error {
	title, err := CleanTitle(title)
	if err != nil {
		return err
	}
	return store.New(s.Pool).RenameCanvas(ctx, store.RenameCanvasParams{ID: cv.ID, Title: title})
}

// Settings are a design's switches.
type Settings = store.GetDesignSettingsRow

// UpdateSettings changes the switches that are set.
func (s *Service) UpdateSettings(ctx context.Context, id uuid.UUID, directEdit, paused *bool) (Settings, error) {
	q := store.New(s.Pool)
	cur, err := q.GetDesignSettings(ctx, id)
	if err != nil {
		return cur, err
	}
	if directEdit != nil {
		cur.DirectEdit = *directEdit
	}
	if paused != nil {
		cur.PersonaPaused = *paused
	}
	if err := q.UpsertDesignSettings(ctx, store.UpsertDesignSettingsParams{CanvasID: id, DirectEdit: cur.DirectEdit, PersonaPaused: cur.PersonaPaused, ProjectID: cur.ProjectID}); err != nil {
		return cur, err
	}
	return cur, nil
}

// ReadAsset checks an uploaded image (PNG or JPEG, 10 MB, 4096 px on a side)
// and keys it under designs/{workspace}/{user}/{sha256}.
func ReadAsset(r io.Reader, ws, user uuid.UUID) (slides.Asset, error) {
	a, err := slides.ReadAsset(r, ws, user)
	if err != nil {
		return a, err
	}
	if a.Width > MaxAssetEdge || a.Height > MaxAssetEdge {
		return a, apperr.Invalid("file", "images are at most %d pixels on a side", MaxAssetEdge)
	}
	a.Key = "designs/" + strings.TrimPrefix(a.Key, "slides/")
	return a, nil
}

// AddAsset lists an image under a design, holding the blob's sweep lock (see
// slides.Service.AddAsset).
func (s *Service) AddAsset(ctx context.Context, p store.CreateDesignAssetParams) (store.DesignAsset, error) {
	var row store.DesignAsset
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockDeckBlob(ctx, p.BlobKey); err != nil {
			return err
		}
		var err error
		row, err = q.CreateDesignAsset(ctx, p)
		return err
	})
	return row, err
}

// ---- proposals ----

// Proposal states and decision states (ADR §3.7).
const (
	StatusPending  = "pending"
	StatusResolved = "resolved"
	StatusApplied  = "applied"

	Accepted = "accepted"
	Rejected = "rejected"
	Stale    = "stale"
)

// Patch is a persona's design_patch.
type Patch struct {
	CanvasID, ChatID uuid.UUID
	RunID            *uuid.UUID
	AgentID          uuid.UUID
	Base             int32
	Summary          string
	Ops              []design.Op
}

// Proposed is what a patch became: a pending proposal, or (direct edit on)
// an applied one with the version it wrote.
type Proposed struct {
	Proposal store.DesignProposal
	Version  *store.ChatCanvasVersion
}

// Propose takes a persona's patch on a design. With direct edit off (the
// default) it dry-runs the ops and holds them as a pending proposal, leaving
// the design untouched; with it on, it applies them as a version by the
// persona and records the proposal as applied. Either way the patch must be
// against the current version (409 otherwise), and a paused design refuses it.
func (s *Service) Propose(ctx context.Context, p Patch) (Proposed, error) {
	var out Proposed
	summary := strings.TrimSpace(p.Summary)
	if n := utf8.RuneCountInString(summary); n < 1 || n > maxSummary {
		return out, apperr.Invalid("summary", "sum the patch up in 1–%d characters", maxSummary)
	}
	if len(p.Ops) == 0 || len(p.Ops) > 100 {
		return out, apperr.Invalid("ops", "provide 1 to 100 ops")
	}
	for i, op := range p.Ops {
		if err := design.CheckOp(i, op, true); err != nil {
			return out, err
		}
	}
	design.MintIDs(p.Ops)
	raw, err := json.Marshal(p.Ops)
	if err != nil {
		return out, err
	}
	err = s.canvases().Locked(ctx, p.ChatID, p.CanvasID, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		if c.Kind != canvas.KindDesign {
			return apperr.Invalid("canvas_id", "isn't a design")
		}
		set, err := q.GetDesignSettings(ctx, c.ID)
		if err != nil {
			return err
		}
		if set.PersonaPaused {
			return apperr.Conflict("the person paused persona edits on this design; wait until they resume")
		}
		if p.Base != cur.Seq {
			return apperr.Conflict("this design is at version %d, not %d; read it again and redo your patch", cur.Seq, p.Base)
		}
		// Apply mutates its ops; keep the stored ones as sent.
		var ops []design.Op
		if err := json.Unmarshal(raw, &ops); err != nil {
			return err
		}
		body, err := design.Apply(cur.Body, ops)
		if err != nil {
			return err
		}
		if body == cur.Body {
			return apperr.Invalid("ops", "these ops change nothing")
		}
		in := store.CreateDesignProposalParams{CanvasID: c.ID, ChatID: &p.ChatID, RunID: p.RunID, AgentID: p.AgentID, BaseSeq: cur.Seq,
			Summary: summary, Ops: raw, Status: StatusPending}
		if set.DirectEdit {
			v, err := canvas.WriteVersion(ctx, q, c, cur, body, summary, canvas.Actor{Type: "agent", ID: p.AgentID})
			if err != nil {
				return err
			}
			out.Version = &v
			in.Status, in.ResultSeq = StatusApplied, &v.Seq
		}
		out.Proposal, err = q.CreateDesignProposal(ctx, in)
		return err
	})
	return out, err
}

// Decision is the person's call on one op of a proposal.
type Decision struct {
	I      int    `json:"i"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// Resolved is a resolved proposal and the version accepting it wrote, if any.
type Resolved struct {
	Proposal store.DesignProposal
	Version  *store.ChatCanvasVersion
}

// Resolve records the person's decision on every op of a pending proposal and
// applies the accepted ones on the current version (not the proposal's base):
// ops are id-keyed, so they rebase. An accepted op that no longer applies (its
// target is gone) is marked stale and skipped, so the person's own later edits
// win. Accepting an op that needs another the person rejected is refused.
func (s *Service) Resolve(ctx context.Context, cv store.GetMyCanvasRow, ws, user, id uuid.UUID, decisions []Decision) (Resolved, error) {
	var out Resolved
	err := s.canvases().Locked(ctx, cv.ChatID, cv.ID, func(q *store.Queries, c store.ChatCanvas, cur store.ChatCanvasVersion) error {
		p, err := q.LockDesignProposal(ctx, store.LockDesignProposalParams{ID: id, CanvasID: c.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		if p.Status != StatusPending {
			return apperr.Conflict("this suggestion was already %s", p.Status)
		}
		var ops []design.Op
		if err := json.Unmarshal(p.Ops, &ops); err != nil {
			return err
		}
		states, err := checkDecisions(ops, decisions)
		if err != nil {
			return err
		}
		var accepted []design.Op
		var at []int
		for i, op := range ops {
			if states[i].State == Accepted {
				accepted, at = append(accepted, op), append(at, i)
			}
		}
		var resultSeq *int32
		if len(accepted) > 0 {
			body, applied, err := design.ApplyEach(cur.Body, accepted)
			if err != nil {
				return err
			}
			n := 0
			for k, ok := range applied {
				if ok {
					n++
				} else {
					states[at[k]].State = Stale
				}
			}
			if n > 0 && body != cur.Body {
				note := fmt.Sprintf("Accepted %d of %d suggested changes", n, len(ops))
				v, err := canvas.WriteVersion(ctx, q, c, cur, body, note, canvas.Actor{Type: "user", ID: user})
				if err != nil {
					return err
				}
				out.Version, resultSeq = &v, &v.Seq
				if _, err := q.MarkMyCanvasReviewed(ctx, store.MarkMyCanvasReviewedParams{ID: c.ID, WorkspaceID: ws, UserID: user, Seq: v.Seq}); err != nil {
					return err
				}
			}
		}
		raw, err := json.Marshal(states)
		if err != nil {
			return err
		}
		out.Proposal, err = q.ResolveDesignProposal(ctx, store.ResolveDesignProposalParams{ID: p.ID, Decisions: raw, ResultSeq: resultSeq})
		return err
	})
	return out, err
}

// checkDecisions wants exactly one accepted or rejected decision per op, so a
// review can't skip one by accident, and no accepted op that needs an op the
// person rejected (one that creates what it refers to).
func checkDecisions(ops []design.Op, ds []Decision) ([]Decision, error) {
	out := make([]Decision, len(ops))
	seen := make([]bool, len(ops))
	for _, d := range ds {
		if d.I < 0 || d.I >= len(ops) {
			return nil, apperr.Invalid("decisions", "op %d isn't in this suggestion (it has %d)", d.I, len(ops))
		}
		if seen[d.I] {
			return nil, apperr.Invalid("decisions", "op %d is decided twice", d.I)
		}
		if d.State != Accepted && d.State != Rejected {
			return nil, apperr.Invalid("decisions", "op %d: state must be accepted or rejected", d.I)
		}
		d.Reason = strings.TrimSpace(d.Reason)
		if utf8.RuneCountInString(d.Reason) > maxReason {
			return nil, apperr.Invalid("decisions", "op %d: a reason is at most %d characters", d.I, maxReason)
		}
		if d.State == Accepted {
			d.Reason = ""
		}
		seen[d.I], out[d.I] = true, d
	}
	for i, ok := range seen {
		if !ok {
			return nil, apperr.Invalid("decisions", "decide op %d too: every change needs accepted or rejected", i)
		}
	}
	for j, deps := range design.Deps(ops) {
		for _, i := range deps {
			if out[j].State == Accepted && out[i].State == Rejected {
				return nil, apperr.Invalid("decisions", "op %d needs op %d, which you rejected; reject op %d too or accept op %d", j, i, j, i)
			}
		}
	}
	return out, nil
}
