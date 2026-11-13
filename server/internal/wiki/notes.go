// Package wiki is the team wiki and agent memory (docs/dev/plan.md §12): notes
// (artifacts of type `note` addressed by path), saving and renaming them,
// agent write permissions and suggestions, backlinks, the link graph,
// knowledge search and the memory a run starts with.
package wiki

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
)

type Service struct {
	Pool      *pgxpool.Pool
	Artifacts *artifacts.Service
	Tracker   *tracker.Service
}

// Namespace lock precedes row locks for every wiki write. This keeps path
// allocation and alias replacement atomic without reversing lock order.
func (s *Service) tx(ctx context.Context, ws uuid.UUID, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockNotePaths(ctx, ws); err != nil {
			return err
		}
		return fn(q)
	})
}

// Write modes for a note save.
const (
	ModeCreate  = "create"  // fail if the page exists
	ModeReplace = "replace" // create or replace the body
	ModeAppend  = "append"  // create, or add to the end of the body
)

// Save is one write to a note.
type Save struct {
	Persona    bool // set only by PersonaWrite
	PersonaID  *uuid.UUID
	ChatID     *uuid.UUID
	RunID      *uuid.UUID
	Title      *string
	Properties artifacts.Properties
	Tags       *[]string
	Aliases    *[]string
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
	if in.Title != nil {
		title, err := artifacts.ValidPageTitle(*in.Title)
		if err != nil {
			return Result{}, err
		}
		in.Title = &title
	}
	if in.Path == "" && in.Title != nil {
		in.Path, err = artifacts.DerivedNotePath(ctx, q, ws, *in.Title)
		if err != nil {
			return Result{}, err
		}
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
		props := in.Properties
		if props == nil {
			props = artifacts.Properties{}
		}
		if in.Tags != nil {
			props["tags"] = *in.Tags
		}
		if in.Aliases != nil {
			props["aliases"] = *in.Aliases
		}
		raw, err := artifacts.NormalizeProperties(props)
		if err != nil {
			return Result{}, err
		}
		title := ""
		if in.Title != nil {
			title = *in.Title
		}
		a, err := s.Artifacts.CreateNoteTx(ctx, q, ws, by, path, in.ProjectKey, in.Body, state, artifacts.NoteMetadata{Title: title, Properties: raw})
		if err != nil {
			return Result{}, err
		}
		a, err = q.GetArtifact(ctx, store.GetArtifactParams{ID: a.ID, WorkspaceID: ws})
		if err != nil {
			return Result{}, err
		}
		v, err := latestVersion(ctx, q, a)
		if err == nil {
			v, err = recordChanges(ctx, q, a, v, in, "", artifacts.Properties{})
		}
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
	if in.Persona {
		if a.PersonaPaused {
			return Result{}, apperr.Conflict("persona edits are paused for this page")
		}
		state = artifacts.Suggested
		if a.PersonaDirect {
			state = artifacts.Published
		}
	}
	cur, curID, err := current(ctx, q, a)
	if err != nil {
		return Result{}, err
	}
	if in.BaseID != nil && curID != nil && *in.BaseID != *curID {
		return Result{}, apperr.Conflict("this page changed since you opened it; reload to see the latest version")
	}
	oldProps := artifacts.Properties{}
	if curID != nil {
		v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *curID, ArtifactID: a.ID, WorkspaceID: ws})
		if err != nil {
			return Result{}, err
		}
		oldProps = artifacts.DecodeProperties(v.Properties)
	}
	oldRaw, _ := artifacts.NormalizeProperties(oldProps)
	props := in.Properties
	if props == nil {
		props = oldProps
	}
	if in.Tags != nil {
		props["tags"] = *in.Tags
	}
	if in.Aliases != nil {
		props["aliases"] = *in.Aliases
	}
	raw, err := artifacts.NormalizeProperties(props)
	if err != nil {
		return Result{}, err
	}

	if in.Title != nil && state == artifacts.Published {
		a, err = artifacts.RenameNoteTitleTx(ctx, q, a, *in.Title)
		if err != nil {
			return Result{}, err
		}
	}
	body := apply(cur, in.Body, mode)
	if body == cur && bytes.Equal(raw, oldRaw) && state == artifacts.Published {
		return Result{Note: a, State: state}, nil // nothing changed: no empty version
	}
	r, err := artifacts.Markdown(body)
	if err != nil {
		return Result{}, err
	}
	v, err := artifacts.AddVersionTx(ctx, q, a, r, by, artifacts.NewVersion{Note: in.Note, State: state, BasedOn: curID, Properties: raw})
	if err != nil {
		return Result{}, err
	}
	v, err = recordChanges(ctx, q, a, v, in, cur, artifacts.DecodeProperties(oldRaw))
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
	err := s.tx(ctx, ws, func(q *store.Queries) error {
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
		err := s.tx(ctx, ws, func(q *store.Queries) error {
			var err error
			res, err = s.write(ctx, q, ws, artifacts.Actor{Type: "agent", ID: agent.ID}, in, artifacts.Suggested)
			return err
		})
		return res, err
	}
	var res Result
	err = s.tx(ctx, ws, func(q *store.Queries) error {
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

// DecideSuggestion retains the legacy whole-suggestion API by resolving all
// remaining changes against the current head.
func (s *Service) DecideSuggestion(ctx context.Context, ws, userID, versionID uuid.UUID, accept bool) (store.Artifact, error) {
	var a store.Artifact
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		v, err := q.LockSuggestion(ctx, store.LockSuggestionParams{ID: versionID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		a, err = pageLock(ctx, q, ws, v.ArtifactID)
		if err != nil {
			return err
		}
		rest := "reject"
		if accept {
			rest = "accept"
		}
		var changes []Change
		if err = s.resolveTx(ctx, q, a, userID, versionID, Resolution{Rest: rest}, &changes); err != nil {
			return err
		}
		a, err = q.GetArtifact(ctx, store.GetArtifactParams{ID: a.ID, WorkspaceID: ws})
		if err != nil {
			return err
		}
		if a.CurrentVersionID == nil {
			remaining, err := q.ListSuggestions(ctx, store.ListSuggestionsParams{WorkspaceID: ws, ArtifactID: &a.ID})
			if err != nil {
				return err
			}
			if len(remaining) == 0 {
				return q.DeleteArtifact(ctx, store.DeleteArtifactParams{ID: a.ID, WorkspaceID: ws})
			}
		}
		return nil
	})
	return a, err
}

// ------------------------------------------------------------ rename

// Rename moves a note to a new path. Links are stored by id so backlinks
// survive; the old path becomes an alias so [[Old name]] still resolves.
func (s *Service) Rename(ctx context.Context, ws, id uuid.UUID, newPath string) (store.Artifact, error) {
	var a store.Artifact
	err := s.tx(ctx, ws, func(q *store.Queries) error { var err error; a, err = renameNote(ctx, q, ws, id, newPath); return err })
	return a, err
}

func renameNote(ctx context.Context, q *store.Queries, ws, id uuid.UUID, newPath string) (store.Artifact, error) {
	var a store.Artifact
	p, err := artifacts.ValidNotePath(newPath)
	if err != nil {
		return a, err
	}
	cur, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && cur.Type != "note") {
		return a, apperr.ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if *cur.Path == p {
		return cur, nil
	}
	if err := q.LockNotePaths(ctx, ws); err != nil {
		return a, err
	}
	if other, err := q.GetNoteByPath(ctx, store.GetNoteByPathParams{WorkspaceID: ws, Path: p}); err == nil && other.ID != cur.ID {
		return a, apperr.Invalid("path", "a page called %q already exists", p)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return a, err
	}
	if other, err := q.FindNoteByAlias(ctx, store.FindNoteByAliasParams{WorkspaceID: ws, Path: p}); err == nil && other.ID != cur.ID {
		return a, apperr.Invalid("path", "%q is another page's alias", p)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return a, err
	}
	if err := q.DeleteAlias(ctx, store.DeleteAliasParams{WorkspaceID: ws, Path: p}); err != nil {
		return a, err
	}
	if !strings.EqualFold(*cur.Path, p) {
		if err := q.AddAlias(ctx, store.AddAliasParams{WorkspaceID: ws, ArtifactID: cur.ID, Path: *cur.Path}); err != nil {
			return a, err
		}
	}
	a, err = q.SetNotePath(ctx, store.SetNotePathParams{ID: cur.ID, Path: &p, Title: cur.Title})
	if err != nil {
		return a, err
	}
	return a, links.ResolvePending(ctx, q, ws)
}

// UpdateAsUser applies a move, title, properties and body together. Reading
// the unchanged body under the row lock prevents metadata-only saves from
// restoring a stale body, and a failed validation rolls the whole edit back.
func (s *Service) UpdateAsUser(ctx context.Context, ws, userID, id uuid.UUID, in Save, newPath, body *string) (Result, error) {
	var res Result
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		a, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.Type != "note") {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if newPath != nil {
			a, err = renameNote(ctx, q, ws, id, *newPath)
			if err != nil {
				return err
			}
		}
		in.Path = *a.Path
		if body == nil {
			in.Body, _, err = current(ctx, q, a)
			if err != nil {
				return err
			}
		} else {
			in.Body = *body
		}
		res, err = s.write(ctx, q, ws, artifacts.Actor{Type: "user", ID: userID}, in, artifacts.Published)
		return err
	})
	return res, err
}
