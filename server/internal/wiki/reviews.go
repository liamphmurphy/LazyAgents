package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/pageview"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type ReviewDecision struct {
	Index  int    `json:"index"`
	Accept bool   `json:"accept"`
	Reason string `json:"reason,omitempty"`
}

func (s *Service) ResolveChanges(ctx context.Context, ws, user, id, vid uuid.UUID, decisions []ReviewDecision) (store.ArtifactVersion, error) {
	if len(decisions) == 0 {
		return store.ArtifactVersion{}, apperr.Invalid("decisions", "select at least one pending change")
	}
	ctx = pageview.Person(ctx, user)
	var out store.ArtifactVersion
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		a, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws, Viewer: &user})
		if err != nil {
			return err
		}
		v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: vid, ArtifactID: id, WorkspaceID: ws, Viewer: &user})
		if err != nil {
			return err
		}
		var changes []artifacts.PageChange
		if err = json.Unmarshal(v.Changes, &changes); err != nil {
			return err
		}
		if len(changes) == 0 {
			return apperr.Conflict("this version has no per-change review")
		}
		direct := v.State == artifacts.Published
		if v.State != artifacts.Suggested && (!direct || v.ReviewState == nil || *v.ReviewState != "pending") {
			return apperr.Conflict("this version was already reviewed")
		}
		for _, d := range decisions {
			if d.Index < 0 || d.Index >= len(changes) || changes[d.Index].State != "pending" {
				return apperr.Invalid("decisions", "unknown or already decided change %d", d.Index)
			}
		}
		body, _, err := current(ctx, q, a)
		if err != nil {
			return err
		}
		before := body
		props := artifacts.Properties{}
		if a.CurrentVersionID != nil {
			cur, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: *a.CurrentVersionID, ArtifactID: id, WorkspaceID: ws, Viewer: &user})
			if err != nil {
				return err
			}
			props = artifacts.DecodeProperties(cur.Properties)
		}
		oldProps, _ := artifacts.NormalizeProperties(props)
		feedback := []string{}
		seen := map[int]bool{}
		for _, d := range decisions {
			if seen[d.Index] {
				return apperr.Invalid("decisions", "duplicate change %d", d.Index)
			}
			seen[d.Index] = true
			c := &changes[d.Index]
			c.Reason = d.Reason
			c.State = "rejected"
			if d.Accept {
				c.State = "accepted"
			}
			if (!direct && d.Accept) || (direct && !d.Accept) {
				var applied bool
				body, applied = artifacts.ApplyPageChange(body, props, *c, direct)
				if !applied {
					c.State = "stale"
					c.Reason = "The page changed; the human edit was preserved."
				}
			}
			if c.State == "rejected" || c.State == "stale" {
				feedback = append(feedback, fmt.Sprintf("Change %d %s: %s", c.Index, c.State, c.Reason))
			}
		}
		raw, err := artifacts.NormalizeProperties(props)
		if err != nil {
			return err
		}
		out = v
		if body != before || string(raw) != string(oldProps) {
			r, err := artifacts.Markdown(body)
			if err != nil {
				return err
			}
			out, err = artifacts.AddVersionTx(ctx, q, a, r, artifacts.Actor{Type: "user", ID: user}, artifacts.NewVersion{Properties: raw, State: artifacts.Published, BasedOn: a.CurrentVersionID, Note: fmt.Sprintf("Reviewed changes from version %s", v.Label)})
			if err != nil {
				return err
			}
		}
		pending, rejected := false, false
		for _, c := range changes {
			pending = pending || c.State == "pending"
			rejected = rejected || c.State == "rejected" || c.State == "stale"
		}
		review := "pending"
		if !pending {
			review = "approved"
			if rejected {
				review = "changes_requested"
			}
		}
		raw, err = json.Marshal(changes)
		if err != nil {
			return err
		}
		if _, err = q.SetPageChanges(ctx, store.SetPageChangesParams{ID: vid, WorkspaceID: ws, Viewer: &user, Changes: raw, ReviewState: &review}); err != nil {
			return err
		}
		if !direct && !pending {
			state := artifacts.Published
			if rejected {
				state = artifacts.Rejected
			}
			if err = q.DecideVersion(ctx, store.DecideVersionParams{ID: vid, State: state, DecidedByID: &user}); err != nil {
				return err
			}
		}
		if a.ChatSessionID != nil && len(feedback) > 0 {
			if _, err = q.LockChatSession(ctx, store.LockChatSessionParams{ID: *a.ChatSessionID, WorkspaceID: ws}); err != nil {
				return err
			}
			_, err = q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: *a.ChatSessionID, Role: "system", BodyMd: "Knowledge review for " + a.Title + ":\n" + strings.Join(feedback, "\n"), ContextRefs: json.RawMessage("[]"), Citations: json.RawMessage("[]")})
			if err != nil {
				return err
			}
		}
		if !direct && !pending && a.CurrentVersionID == nil && out.ID == v.ID {
			remaining, err := q.ListSuggestions(ctx, store.ListSuggestionsParams{WorkspaceID: ws, ArtifactID: &a.ID, Viewer: &user})
			if err != nil {
				return err
			}
			if len(remaining) == 0 {
				return q.DeleteArtifact(ctx, store.DeleteArtifactParams{ID: a.ID, WorkspaceID: ws})
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) RestorePage(ctx context.Context, ws, user, id, vid uuid.UUID) (Result, error) {
	ctx = pageview.Person(ctx, user)
	var out Result
	err := s.tx(ctx, ws, func(q *store.Queries) error {
		a, err := q.LockArtifact(ctx, store.LockArtifactParams{ID: id, WorkspaceID: ws, Viewer: &user})
		if err != nil {
			return err
		}
		v, err := q.GetArtifactVersion(ctx, store.GetArtifactVersionParams{ID: vid, ArtifactID: id, WorkspaceID: ws, Viewer: &user})
		if err != nil {
			return err
		}
		if v.BodyMd == nil {
			return apperr.Invalid("version", "not a text page")
		}
		out, err = s.write(ctx, q, ws, artifacts.Actor{Type: "user", ID: user}, Save{PageID: &id, Path: *a.Path, Body: *v.BodyMd, Properties: artifacts.DecodeProperties(v.Properties), Note: "Restored " + v.Label}, artifacts.Published)
		return err
	})
	return out, err
}
