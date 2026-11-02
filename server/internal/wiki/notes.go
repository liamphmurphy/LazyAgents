// Package wiki is the team wiki and agent memory (PLAN.md §12): notes
// (artifacts of type `note` addressed by path), saving and renaming them,
// agent write permissions and suggestions, backlinks, the link graph,
// knowledge search and the memory a run starts with.
package wiki

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/artifacts"
	"github.com/liamphmurphy/lazyagents/server/internal/links"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
	"github.com/liamphmurphy/lazyagents/server/internal/tracker"
)

type Service struct {
	Pool      *pgxpool.Pool
	Artifacts *artifacts.Service
	Tracker   *tracker.Service
}

func (s *Service) tx(ctx context.Context, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error { return fn(store.New(tx)) })
}

// Write modes for a note save.
const (
	ModeCreate  = "create"  // fail if the page exists
	ModeReplace = "replace" // create or replace the body
	ModeAppend  = "append"  // create, or add to the end of the body
)

// Save is one write to a note.
type Save struct {
	Path       string
	Body       string
	Mode       string     // default replace
	ProjectKey string     // new notes only; "" = workspace-wide
	Note       string     // version note
	BaseID     *uuid.UUID // the version the editor started from; stale saves are refused
}

// Result says what a save did.
type Result struct {
	Note    store.Artifact
	Version store.ArtifactVersion
	Created bool
	State   string // published or suggested
}

func apply(cur, body, mode string) string {
	if mode == ModeAppend && cur != "" {
		return strings.TrimRight(cur, "\n") + "\n\n" + strings.TrimLeft(body, "\n")
	}
	return body
}

func checkMode(m string) (string, error) {
	switch m {
	case "":
		return ModeReplace, nil
	case ModeCreate, ModeReplace, ModeAppend:
		return m, nil
	}
	return "", apperr.Invalid("mode", "must be create, replace or append")
}

// current returns a note's current body ("" for a page that only has a
// suggestion so far).
func current(ctx context.Context, q *store.Queries, a store.Artifact) (string, *uuid.UUID, error) {
	if a.CurrentVersionID == nil {
		return "", nil, nil
	}
	v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID})
	if err != nil || v.BodyMd == nil {
		return "", nil, err
	}
	return *v.BodyMd, &v.ID, nil
}

// write saves a note as `by`, either publishing or as a suggestion.
func (s *Service) write(ctx context.Context, q *store.Queries, ws uuid.UUID, by artifacts.Actor, in Save, state string) (Result, error) {
	mode, err := checkMode(in.Mode)
	if err != nil {
		return Result{}, err
	}
	path, err := artifacts.ValidNotePath(in.Path)
	if err != nil {
		return Result{}, err
	}
	a, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: path})
	// Saving to a renamed page's old path saves the page; creating there
	// makes a new page that takes the path over.
	if errors.Is(err, pgx.ErrNoRows) && mode != ModeCreate {
		a, err = q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: ws, Path: path})
		if err == nil {
			path = *a.Path // writing to an old name writes to the renamed page
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		a, err := s.Artifacts.CreateNoteTx(ctx, q, ws, by, path, in.ProjectKey, in.Body, state)
		if err != nil {
			return Result{}, err
		}
		v, err := latestVersion(ctx, q, a)
		return Result{Note: a, Version: v, Created: true, State: state}, err
	} else if err != nil {
		return Result{}, err
	}
	if mode == ModeCreate && a.CurrentVersionID != nil {
		return Result{}, apperr.Invalid("path", "a page called %q already exists; use mode replace or append", *a.Path)
	}
	if a, err = q.LockArtifact(ctx, store.LockArtifactParams{ID: a.ID, WorkspaceID: ws}); err != nil {
		return Result{}, err
	}
	cur, curID, err := current(ctx, q, a)
	if err != nil {
		return Result{}, err
	}
	if in.BaseID != nil && curID != nil && *in.BaseID != *curID {
		return Result{}, apperr.Conflict("this page changed since you opened it; reload to see the latest version")
	}
	body := apply(cur, in.Body, mode)
	if body == cur && state == artifacts.Published {
		return Result{Note: a, State: state}, nil // nothing changed: no empty version
	}
	r, err := artifacts.Markdown(body)
	if err != nil {
		return Result{}, err
	}
	v, err := artifacts.AddVersionTx(ctx, q, a, r, by, artifacts.NewVersion{Note: in.Note, State: state, BasedOn: curID})
	if err != nil {
		return Result{}, err
	}
	a, err = q.GetArtifact(ctx, store.GetArtifactParams{ID: a.ID, WorkspaceID: ws})
	return Result{Note: a, Version: v, State: state}, err
}

func latestVersion(ctx context.Context, q *store.Queries, a store.Artifact) (store.ArtifactVersion, error) {
	if a.CurrentVersionID != nil {
		return q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID})
	}
	sugg, err := q.ListSuggestions(ctx, store.ListSuggestionsParams{WorkspaceID: a.WorkspaceID, ArtifactID: &a.ID})
	if err != nil || len(sugg) == 0 {
		return store.ArtifactVersion{}, err
	}
	last := sugg[len(sugg)-1]
	return q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: last.ID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID})
}

// SaveAsUser publishes a person's edit directly (the lightweight wiki save:
// no "New version" ceremony).
func (s *Service) SaveAsUser(ctx context.Context, ws, userID uuid.UUID, in Save) (Result, error) {
	var res Result
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		res, err = s.write(ctx, q, ws, artifacts.Actor{Type: "user", ID: userID}, in, artifacts.Published)
		return err
	})
	return res, err
}

// ------------------------------------------------------------ agent writes

// MemoryRoot is an agent's namespace: agents/<handle>/.
func MemoryRoot(handle string) string { return "agents/" + handle + "/" }

// MemoryIndex is the page listing an agent's notes.
func MemoryIndex(handle string) string { return MemoryRoot(handle) + "Memory" }

// Decision is how an agent's write to a path is handled (§12.5).
type Decision string

const (
	Direct  Decision = "direct"
	Suggest Decision = "suggest"
	Refuse  Decision = "refuse"
)

// Decide applies §12.5: an agent's own namespace is always writable;
// another agent's never is; everything else follows wiki:write. A
// definition that doesn't mention wiki:write gets suggestions.
func Decide(handle, path string, mode agentdef.Mode, set bool) Decision {
	lp := strings.ToLower(path)
	if strings.HasPrefix(lp, strings.ToLower(MemoryRoot(handle))) {
		return Direct
	}
	if strings.HasPrefix(lp, "agents/") {
		return Refuse
	}
	if !set {
		return Suggest
	}
	switch mode {
	case agentdef.Allowed:
		return Direct
	case agentdef.RequiresApproval:
		return Suggest
	}
	return Refuse
}

// AgentWrite saves a note as an agent, honouring wiki:write. The scope
// comes from the run's frozen definition when there is one.
func (s *Service) AgentWrite(ctx context.Context, agent store.Agent, scopes map[string]agentdef.Mode, in Save) (Result, error) {
	ws := agent.WorkspaceID
	path, err := artifacts.ValidNotePath(in.Path)
	if err != nil {
		return Result{}, err
	}
	in.Path = path
	mode, set := agentdef.ModeFor(scopes, "wiki:write")
	switch Decide(agent.Handle, path, mode, set) {
	case Refuse:
		if strings.HasPrefix(strings.ToLower(path), "agents/") {
			return Result{}, apperr.Invalid("path", "you can only write under %s (your memory), not another agent's", MemoryRoot(agent.Handle))
		}
		return Result{}, apperr.Invalid("path", "wiki:write is denied for you: write under %s instead", MemoryRoot(agent.Handle))
	case Suggest:
		in.BaseID = nil
		var res Result
		err := s.tx(ctx, func(q *store.Queries) error {
			var err error
			res, err = s.write(ctx, q, ws, artifacts.Actor{Type: "agent", ID: agent.ID}, in, artifacts.Suggested)
			return err
		})
		return res, err
	}
	var res Result
	err = s.tx(ctx, func(q *store.Queries) error {
		var err error
		if res, err = s.write(ctx, q, ws, artifacts.Actor{Type: "agent", ID: agent.ID}, in, artifacts.Published); err != nil {
			return err
		}
		return s.addToIndex(ctx, q, agent, res.Note)
	})
	return res, err
}

// addToIndex lists a new memory note on the agent's Memory page, creating
// the page the first time.
func (s *Service) addToIndex(ctx context.Context, q *store.Queries, agent store.Agent, note store.Artifact) error {
	index := MemoryIndex(agent.Handle)
	if note.Path == nil || !strings.HasPrefix(strings.ToLower(*note.Path), strings.ToLower(MemoryRoot(agent.Handle))) ||
		strings.EqualFold(*note.Path, index) {
		return nil
	}
	by := artifacts.Actor{Type: "agent", ID: agent.ID}
	line := "- [[" + *note.Path + "]]"
	idx, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: agent.WorkspaceID, Path: index})
	if errors.Is(err, pgx.ErrNoRows) {
		body := fmt.Sprintf("# %s's memory\n\nNotes %s keeps while working: gotchas, decisions and dead ends worth remembering next time.\n\n%s\n", agent.Name, agent.Name, line)
		_, err = s.Artifacts.CreateNoteTx(ctx, q, agent.WorkspaceID, by, index, "", body, artifacts.Published)
		return err
	} else if err != nil {
		return err
	}
	linked, err := q.NoteLinksTo(ctx, store.NoteLinksToParams{SourceID: idx.ID, TargetID: &note.ID})
	if err != nil || linked {
		return err
	}
	_, err = s.write(ctx, q, agent.WorkspaceID, by, Save{Path: index, Body: line, Mode: ModeAppend, Note: "Listed " + links.TitleOf(*note.Path)}, artifacts.Published)
	return err
}

// ------------------------------------------------------------ suggestions

// Decide accepts or rejects a suggested version. Accepting publishes it as
// the next version, even if the page moved on since (the UI warns).
func (s *Service) DecideSuggestion(ctx context.Context, ws, userID, versionID uuid.UUID, accept bool) (store.Artifact, error) {
	var a store.Artifact
	err := s.tx(ctx, func(q *store.Queries) error {
		v, err := q.LockSuggestion(ctx, store.LockSuggestionParams{ID: versionID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		if v.State != artifacts.Suggested {
			return apperr.Conflict("this suggestion was already %s", v.State)
		}
		if a, err = q.LockArtifact(ctx, store.LockArtifactParams{ID: v.ArtifactID, WorkspaceID: ws}); err != nil {
			return err
		}
		if !accept {
			if err := q.DecideVersion(ctx, store.DecideVersionParams{ID: v.ID, State: artifacts.Rejected, DecidedByID: &userID}); err != nil {
				return err
			}
			// A page that only ever existed as this suggestion goes away.
			if a.CurrentVersionID == nil {
				rest, err := q.ListSuggestions(ctx, store.ListSuggestionsParams{WorkspaceID: ws, ArtifactID: &a.ID})
				if err != nil {
					return err
				}
				if len(rest) == 0 {
					return q.DeleteArtifact(ctx, store.DeleteArtifactParams{ID: a.ID, WorkspaceID: ws})
				}
			}
			return nil
		}
		n, err := q.CountPublishedVersions(ctx, a.ID)
		if err != nil {
			return err
		}
		if err := q.RelabelVersion(ctx, store.RelabelVersionParams{ID: v.ID, Label: fmt.Sprintf("v%d", n+1)}); err != nil {
			return err
		}
		if err := q.DecideVersion(ctx, store.DecideVersionParams{ID: v.ID, State: artifacts.Published, DecidedByID: &userID}); err != nil {
			return err
		}
		first := a.CurrentVersionID == nil
		if err := artifacts.Publish(ctx, q, a, v); err != nil {
			return err
		}
		if first {
			if err := links.ResolvePending(ctx, q, ws); err != nil {
				return err
			}
		}
		a, err = q.GetArtifact(ctx, store.GetArtifactParams{ID: a.ID, WorkspaceID: ws})
		return err
	})
	return a, err
}

// ------------------------------------------------------------ rename

// Rename moves a note to a new path. Links are stored by id so backlinks
// survive; the old path becomes an alias so [[Old name]] still resolves.
func (s *Service) Rename(ctx context.Context, ws, id uuid.UUID, newPath string) (store.Artifact, error) {
	var a store.Artifact
	err := s.tx(ctx, func(q *store.Queries) error {
		p, err := artifacts.ValidNotePath(newPath)
		if err != nil {
			return err
		}
		cur, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && cur.Type != "note") {
			return apperr.ErrNotFound
		} else if err != nil {
			return err
		}
		if *cur.Path == p {
			a = cur
			return nil
		}
		if other, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: p}); err == nil && other.ID != cur.ID {
			return apperr.Invalid("path", "a page called %q already exists", p)
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := q.DeleteAlias(ctx, store.DeleteAliasParams{WorkspaceID: ws, Path: p}); err != nil {
			return err
		}
		if !strings.EqualFold(*cur.Path, p) {
			if err := q.AddAlias(ctx, store.AddAliasParams{WorkspaceID: ws, ArtifactID: cur.ID, Path: *cur.Path}); err != nil {
				return err
			}
		}
		if a, err = q.SetNotePath(ctx, store.SetNotePathParams{ID: cur.ID, Path: &p, Title: links.TitleOf(p)}); err != nil {
			return err
		}
		return links.ResolvePending(ctx, q, ws)
	})
	return a, err
}
