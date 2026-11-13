package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type ChangeDecision struct {
	I      int    `json:"i"`
	Accept bool   `json:"accept"`
	Reason string `json:"reason,omitempty"`
}
type Resolution struct {
	Decisions []ChangeDecision `json:"decisions"`
	Rest      string           `json:"rest,omitempty"`
	Reason    string           `json:"reason,omitempty"`
}

func pageLock(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (store.Artifact, error) {
	a, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && a.Type != "note") {
		return a, apperr.ErrNotFound
	}
	return a, err
}

func (s *Service) SetPersonaEdits(ctx context.Context, ws, id uuid.UUID, direct, paused *bool) error {
	return s.tx(ctx, ws, func(q *store.Queries) error {
		if _, err := pageLock(ctx, q, ws, id); err != nil {
			return err
		}
		return q.SetPagePersonaEdits(ctx, store.SetPagePersonaEditsParams{ID: id, WorkspaceID: ws, Direct: direct, Paused: paused})
	})
}

// RestoreVersion copies the old snapshot; history and authors stay intact.
func (s *Service) RestoreVersion(ctx context.Context, ws, user, id, vid uuid.UUID) (store.Artifact, error) {
	var a store.Artifact
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		var err error
		a, err = pageLock(ctx, q, ws, id)
		if err != nil {
			return err
		}
		v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: vid, ArtifactID: id, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if v.State != artifacts.Published {
			return apperr.Invalid("version", "only published versions can be restored")
		}
		r, err := artifacts.Markdown(value(v.BodyMd))
		if err != nil {
			return err
		}
		_, err = artifacts.AddVersionTx(ctx, q, a, r, artifacts.Actor{Type: "user", ID: user}, artifacts.NewVersion{Properties: v.Properties, BasedOn: a.CurrentVersionID, Note: "Restored " + v.Label})
		if err != nil {
			return err
		}
		a, err = q.GetArtifact(ctx, store.GetArtifactParams{ID: id, WorkspaceID: ws})
		return err
	})
	return a, err
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// versionChanges also handles versions written before the changes column existed.
func versionChanges(ctx context.Context, q *store.Queries, a store.Artifact, v store.ArtifactVersion) ([]Change, error) {
	var changes []Change
	if len(v.Changes) > 0 {
		err := json.Unmarshal(v.Changes, &changes)
		return changes, err
	}
	base := ""
	props := artifacts.Properties{}
	if v.BasedOnVersionID != nil {
		b, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *v.BasedOnVersionID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID})
		if err != nil {
			return nil, err
		}
		base = value(b.BodyMd)
		props = artifacts.DecodeProperties(b.Properties)
	}
	return diffChanges(base, value(v.BodyMd), props, artifacts.DecodeProperties(v.Properties), v.Note), nil
}

func saveChanges(ctx context.Context, q *store.Queries, v store.ArtifactVersion, changes []Change, review *string) (store.ArtifactVersion, error) {
	raw, err := json.Marshal(changes)
	if err != nil {
		return v, err
	}
	return q.SetPageVersionChanges(ctx, store.SetPageVersionChangesParams{ID: v.ID, Changes: raw, PersonaID: v.PersonaID, SourceChatID: v.SourceChatID, SourceRunID: v.SourceRunID, ReviewState: review})
}

func (s *Service) ResolveSuggestion(ctx context.Context, ws, user, id, vid uuid.UUID, in Resolution) ([]Change, error) {
	var result []Change
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		a, err := pageLock(ctx, q, ws, id)
		if err != nil {
			return err
		}
		return s.resolveTx(ctx, q, a, user, vid, in, &result)
	})
	return result, err
}

func (s *Service) resolveTx(ctx context.Context, q *store.Queries, a store.Artifact, user, vid uuid.UUID, in Resolution, result *[]Change) error {
	v, err := q.LockSuggestion(ctx, store.LockSuggestionParams{ID: vid, WorkspaceID: a.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && v.ArtifactID != a.ID) {
		return apperr.ErrNotFound
	}
	if err != nil {
		return err
	}
	direct := v.State == artifacts.Published && v.ReviewState != nil && *v.ReviewState == "pending" && len(v.Changes) > 0
	if v.State != artifacts.Suggested && !direct {
		return apperr.Conflict("this suggestion was already resolved")
	}
	changes, err := versionChanges(ctx, q, a, v)
	if err != nil {
		return err
	}
	if in.Rest != "" && in.Rest != "accept" && in.Rest != "reject" {
		return apperr.Invalid("rest", "must be accept or reject")
	}
	decisions := map[int]ChangeDecision{}
	for _, d := range in.Decisions {
		if d.I < 0 || d.I >= len(changes) {
			return apperr.Invalid("decisions", "unknown change %d", d.I)
		}
		if _, ok := decisions[d.I]; ok {
			return apperr.Invalid("decisions", "duplicate change %d", d.I)
		}
		if changes[d.I].State != "pending" && changes[d.I].State != "applied" {
			return apperr.Conflict("change %d was already resolved", d.I)
		}
		decisions[d.I] = d
	}
	for i, c := range changes {
		if _, ok := decisions[i]; !ok && in.Rest != "" && (c.State == "pending" || c.State == "applied") {
			decisions[i] = ChangeDecision{I: i, Accept: in.Rest == "accept", Reason: in.Reason}
		}
	}
	if len(decisions) == 0 && len(changes) > 0 {
		return apperr.Invalid("decisions", "select at least one pending change")
	}
	body, _, err := current(ctx, q, a)
	if err != nil {
		return err
	}
	props := artifacts.Properties{}
	if a.CurrentVersionID != nil {
		cur, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: a.ID, WorkspaceID: a.WorkspaceID})
		if err != nil {
			return err
		}
		props = artifacts.DecodeProperties(cur.Properties)
	}
	type patch struct {
		start, end int
		text       string
	}
	patches := []patch{}
	feedback := []string{}
	accepted := 0
	mutated := false
	now := time.Now().UTC()
	for i := range changes {
		d, ok := decisions[i]
		if !ok {
			continue
		}
		c := &changes[i]
		c.DecidedBy = &user
		c.DecidedAt = &now
		c.Reason = d.Reason
		if d.Accept {
			c.State = "accepted"
		} else {
			c.State = "rejected"
			if c.Reason == "" {
				c.Reason = "Rejected by the person"
			}
		}
		// Suggested accepts apply forwards; direct-edit rejects apply the inverse.
		if d.Accept != direct {
			target := *c
			if direct {
				target.Before, target.After = c.After, c.Before
			}
			valid := true
			if target.Kind == "property" {
				valid = equalValue(props[target.Key], target.Before)
				if valid {
					if target.After == nil {
						delete(props, target.Key)
					} else {
						props[target.Key] = target.After
					}
					mutated = true
				}
			} else {
				start, end, ok := locate(body, target)
				valid = ok
				if valid {
					for _, p := range patches {
						if start < p.end && end > p.start || start == p.start {
							valid = false
							break
						}
					}
				}
				if valid {
					patches = append(patches, patch{start, end, target.After.(string)})
					mutated = true
				}
			}
			if !valid {
				c.State = "stale"
				c.Reason = "Target changed or is ambiguous in the current page; the person's edit was preserved"
			}
		}
		if c.State == "accepted" {
			accepted++
		}
		if c.State == "rejected" || c.State == "stale" {
			target, _ := json.Marshal(map[string]any{"kind": c.Kind, "key": c.Key, "before": c.Before, "after": c.After})
			feedback = append(feedback, fmt.Sprintf("Change %d (%s): %s — %s\n%s", c.I, c.Why, c.State, c.Reason, target))
		}
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start > patches[j].start })
	for _, p := range patches {
		body = body[:p.start] + p.text + body[p.end:]
	}
	if mutated {
		raw, err := artifacts.NormalizeProperties(props)
		if err != nil {
			return err
		}
		r, err := artifacts.Markdown(body)
		if err != nil {
			return err
		}
		note := fmt.Sprintf("Accepted %d of %d changes", accepted, len(changes))
		if v.PersonaID != nil && v.SourceChatID != nil {
			chat, e := q.GetChatSession(ctx, store.GetChatSessionParams{ID: *v.SourceChatID, WorkspaceID: a.WorkspaceID})
			if e != nil {
				return e
			}
			persona, e := q.GetPersona(ctx, store.GetPersonaParams{ID: *v.PersonaID, WorkspaceID: a.WorkspaceID, UserID: chat.CreatedBy})
			if e != nil {
				return e
			}
			note += " from " + persona.Name
		} else if v.CreatedByType == "agent" {
			author, e := q.GetAgent(ctx, store.GetAgentParams{ID: v.CreatedByID, WorkspaceID: a.WorkspaceID})
			if e == nil {
				note += " from @" + author.Handle
			}
		}
		if _, err = artifacts.AddVersionTx(ctx, q, a, r, artifacts.Actor{Type: "user", ID: user}, artifacts.NewVersion{Properties: raw, BasedOn: a.CurrentVersionID, Note: note}); err != nil {
			return err
		}
	}
	pending, allAccepted := false, true
	for _, c := range changes {
		pending = pending || c.State == "pending" || c.State == "applied"
		allAccepted = allAccepted && c.State == "accepted"
	}
	review := "pending"
	if !pending {
		review = "approved"
	}
	if _, err = saveChanges(ctx, q, v, changes, &review); err != nil {
		return err
	}
	if !pending && !direct {
		state := artifacts.Rejected
		if allAccepted {
			state = artifacts.Published
		}
		if err = q.DecideVersion(ctx, store.DecideVersionParams{ID: v.ID, State: state, DecidedByID: &user}); err != nil {
			return err
		}
	}
	v.DecidedByID = &user
	if len(feedback) > 0 {
		if err = s.suggestionFeedback(ctx, q, a, v, strings.Join(feedback, "\n")); err != nil {
			return err
		}
	}
	*result = changes
	return nil
}

func (s *Service) suggestionFeedback(ctx context.Context, q *store.Queries, a store.Artifact, v store.ArtifactVersion, body string) error {
	body = "Page review: " + a.Title + "\n\n" + body
	chat := v.SourceChatID
	if chat == nil && v.SourceRunID == nil {
		chat = a.ChatSessionID
	}
	if chat != nil {
		if _, err := q.LockChatSessionByID(ctx, *chat); err != nil {
			return err
		}
		_, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: *chat, Role: "system", BodyMd: body, ContextRefs: json.RawMessage(`[]`), Citations: json.RawMessage(`[]`), PersonaID: v.PersonaID})
		return err
	}
	if v.SourceRunID != nil {
		run, err := q.GetRun(ctx, store.GetRunParams{ID: *v.SourceRunID, WorkspaceID: a.WorkspaceID})
		if err != nil {
			return err
		}
		if run.TicketID != nil {
			_, err = q.CreateComment(ctx, store.CreateCommentParams{WorkspaceID: a.WorkspaceID, TicketID: *run.TicketID, AuthorType: "user", AuthorID: *v.DecidedByID, BodyMd: body})
			return err
		}
	}
	return nil
}

func recordChanges(ctx context.Context, q *store.Queries, a store.Artifact, v store.ArtifactVersion, in Save, base string, old artifacts.Properties) (store.ArtifactVersion, error) {
	if v.State != artifacts.Suggested && !in.Persona {
		return v, nil
	}
	why := in.Note
	if why == "" {
		why = "Proposed page edit"
	}
	changes := diffChanges(base, value(v.BodyMd), old, artifacts.DecodeProperties(v.Properties), why)
	var review *string
	if in.Persona && v.State == artifacts.Published {
		pending := "pending"
		review = &pending
		for i := range changes {
			changes[i].State = "applied"
		}
	}
	v.PersonaID = in.PersonaID
	v.SourceChatID = in.ChatID
	v.SourceRunID = in.RunID
	if in.ChatID != nil && a.ChatSessionID == nil {
		if err := q.SetPageChat(ctx, store.SetPageChatParams{ID: a.ID, ChatSessionID: in.ChatID}); err != nil {
			return v, err
		}
	}
	return saveChanges(ctx, q, v, changes, review)
}

// PersonaWrite uses the page's controls, independent of a ticket run's scopes.
// The chat identifies the person and persona; callers cannot supply attribution.
func (s *Service) PersonaWrite(ctx context.Context, agent store.Agent, chatID uuid.UUID, runID *uuid.UUID, in Save) (Result, error) {
	var res Result
	if strings.HasPrefix(strings.ToLower(in.Path), "agents/") && !strings.HasPrefix(strings.ToLower(in.Path), strings.ToLower(MemoryRoot(agent.Handle))) {
		return res, apperr.Invalid("path", "you can only write under %s, not another agent's memory", MemoryRoot(agent.Handle))
	}
	err := s.tx(ctx, agent.WorkspaceID, func(q *store.Queries) error {
		chat, err := q.GetChatSession(ctx, store.GetChatSessionParams{ID: chatID, WorkspaceID: agent.WorkspaceID})
		if err != nil {
			return err
		}
		if chat.AgentID != agent.ID {
			return apperr.ErrNotFound
		}
		in.Persona = true
		in.ChatID = &chat.ID
		in.RunID = runID
		in.PersonaID = chat.SpeakerPersonaID
		if in.PersonaID == nil {
			in.PersonaID = chat.PersonaID
		}
		if in.PersonaID != nil {
			if _, err = q.GetPersona(ctx, store.GetPersonaParams{ID: *in.PersonaID, WorkspaceID: agent.WorkspaceID, UserID: chat.CreatedBy}); err != nil {
				return err
			}
		}
		res, err = s.write(ctx, q, agent.WorkspaceID, artifacts.Actor{Type: "agent", ID: agent.ID}, in, artifacts.Suggested)
		return err
	})
	return res, err
}
