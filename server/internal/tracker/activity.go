package tracker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// Activity verbs and their payloads. Payloads carry display names so the
// timeline renders without extra lookups and survives renames as history.
const (
	VerbCreated            = "created"            // {}
	VerbStatusChanged      = "status_changed"     // {from, to}
	VerbPriorityChanged    = "priority_changed"   // {from, to}
	VerbTitleChanged       = "title_changed"      // {from, to}
	VerbDescriptionEdited  = "description_edited" // {}
	VerbAssigned           = "assigned"           // {agent_id, agent_name}
	VerbUnassigned         = "unassigned"         // {agent_id, agent_name}
	VerbReviewerSet        = "reviewer_set"       // {user_id, user_name}
	VerbReviewerCleared    = "reviewer_cleared"   // {}
	VerbEpicChanged        = "epic_changed"       // {epic_id?, epic_name?}
	VerbLabelsChanged      = "labels_changed"     // {added: [name], removed: [name]}
	VerbEstimateChanged    = "estimate_changed"   // {to?}
	VerbDueChanged         = "due_changed"        // {to?} (YYYY-MM-DD)
	VerbCriterionAdded     = "criterion_added"    // {criterion_id, text}
	VerbCriterionChecked   = "criterion_checked"  // {criterion_id, text}
	VerbCriterionUnchecked = "criterion_unchecked"
	VerbCriterionRemoved   = "criterion_removed" // {text}
	VerbCommented          = "commented"         // {comment_id, body_md, parent_comment_id?}
	VerbArtifactAttached   = "artifact_attached" // {artifact_id, title, type, relation}
	VerbArtifactDetached   = "artifact_detached" // {artifact_id, title, type}
)

// Actor is who caused a change.
type Actor struct {
	Type string // user | agent | system
	ID   *uuid.UUID
}

func UserActor(id uuid.UUID) Actor { return Actor{Type: "user", ID: &id} }

type event struct {
	verb    string
	payload map[string]any
	at      *time.Time // seeds backdate events; nil = now()
}

func record(ctx context.Context, q *store.Queries, ws uuid.UUID, ticketID uuid.UUID, epicID *uuid.UUID, a Actor, e event) error {
	_, err := insert(ctx, q, ws, ticketID, epicID, a, e)
	return err
}

func insert(ctx context.Context, q *store.Queries, ws uuid.UUID, ticketID uuid.UUID, epicID *uuid.UUID, a Actor, e event) (store.ActivityEvent, error) {
	p := e.payload
	if p == nil {
		p = map[string]any{}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return store.ActivityEvent{}, err
	}
	return q.InsertActivity(ctx, store.InsertActivityParams{
		WorkspaceID: ws,
		TicketID:    &ticketID,
		EpicID:      epicID,
		ActorType:   a.Type,
		ActorID:     a.ID,
		Verb:        e.verb,
		Payload:     raw,
		CreatedAt:   e.at,
	})
}
