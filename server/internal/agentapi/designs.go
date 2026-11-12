package agentapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/canvas"
	"github.com/liamphmurphy/vesper/server/internal/design"
	"github.com/liamphmurphy/vesper/server/internal/designs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type DesignFlowsIn struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	Page     string    `json:"page,omitempty" jsonschema:"A page id or name; every page when omitted"`
}

// designFlows is design_flows (ADR-designs §3.9): a design's prototype as
// {frames, links} plus a Mermaid flowchart.
func designFlows(ctx context.Context, d Deps, c agents.Caller, in DesignFlowsIn) (any, error) {
	s, err := canvasScope(ctx, d, c, "design_flows")
	if err != nil {
		return nil, err
	}
	cv, err := getCanvas(ctx, d, s, in.CanvasID)
	if err != nil {
		return nil, err
	}
	if cv.Kind != canvas.KindDesign {
		return nil, apperr.Invalid("canvas_id", "isn't a design")
	}
	v, err := d.Q.LatestChatCanvasVersion(ctx, cv.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	doc, err := design.Parse(v.Body)
	if err != nil {
		return nil, err
	}
	f, err := design.FlowsOf(doc, in.Page)
	if err != nil {
		return nil, apperr.Invalid("page", "%s doesn't exist", in.Page)
	}
	return map[string]any{"canvas_id": cv.ID, "version": v.Seq, "frames": f.Frames, "links": f.Links, "broken_links": f.Broken, "mermaid": f.Mermaid}, nil
}

type DesignComponentsIn struct {
	CanvasID uuid.UUID `json:"canvas_id"`
}

// designComponents is design_components: the design's main components and
// their instances.
func designComponents(ctx context.Context, d Deps, c agents.Caller, in DesignComponentsIn) (any, error) {
	_, cv, v, err := designTarget(ctx, d, c, "design_components", in.CanvasID)
	if err != nil {
		return nil, err
	}
	doc, err := design.Parse(v.Body)
	if err != nil {
		return nil, err
	}
	return map[string]any{"canvas_id": cv.ID, "version": v.Seq, "components": design.Components(doc),
		"hint": "Editing a component changes every instance; overrides (text, fill, visible) are keyed by the component's descendant ids. Use instantiate rather than copying a component's nodes."}, nil
}

type DesignTokensIn struct {
	CanvasID uuid.UUID `json:"canvas_id"`
}

// designTokens is design_tokens: the token sets, the one in use, the tokens
// the design uses, and whether tokens.css changed since its last sync.
func designTokens(ctx context.Context, d Deps, c agents.Caller, in DesignTokensIn) (any, error) {
	_, cv, v, err := designTarget(ctx, d, c, "design_tokens", in.CanvasID)
	if err != nil {
		return nil, err
	}
	doc, err := design.Parse(v.Body)
	if err != nil {
		return nil, err
	}
	t := design.TokensOf(doc)
	out := map[string]any{"canvas_id": cv.ID, "version": v.Seq, "token_sets": t.Sets, "tokens_ref": t.Ref, "used": t.Used, "tokens_css_changed": t.TokensCSSChange}
	if t.TokensCSSChange {
		out["note"] = "tokens.css has changed since this design's set was synced. Only the person can sync (the badge in the editor); the design keeps its snapshot until then."
	}
	return out, nil
}

type DesignInspectIn struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	NodeID   string    `json:"node_id" jsonschema:"The node (n_…) to inspect, usually a frame"`
	Target   string    `json:"target,omitempty" jsonschema:"css, tailwind or react; react when omitted"`
}

// designInspect is design_inspect: the node as code, with spacing, type
// styles, token names and assets, computed by the design worker.
func designInspect(ctx context.Context, d Deps, c agents.Caller, in DesignInspectIn) (any, error) {
	_, cv, v, err := designTarget(ctx, d, c, "design_inspect", in.CanvasID)
	if err != nil {
		return nil, err
	}
	if err := requireDesigns(d); err != nil {
		return nil, err
	}
	if in.Target == "" {
		in.Target = "react"
	}
	if in.Target != "css" && in.Target != "tailwind" && in.Target != "react" {
		return nil, apperr.Invalid("target", "use css, tailwind or react")
	}
	if !design.HasNode(v.Body, in.NodeID) {
		return nil, apperr.Invalid("node_id", "no node %s in this design", in.NodeID)
	}
	out, err := d.Designs.Inspect(ctx, v.Body, in.NodeID, in.Target)
	var rejected *designs.RejectedError
	switch {
	case errors.As(err, &rejected):
		return nil, apperr.Invalid("node_id", "%s", rejected.Message)
	case errors.Is(err, designs.ErrUnavailable):
		return map[string]any{"inspect": "unavailable", "message": "inspect unavailable: the design renderer isn't reachable right now; carry on from design_read"}, nil
	case err != nil:
		return nil, err
	}
	out["canvas_id"] = cv.ID
	out["version"] = v.Seq
	return out, nil
}

type DesignLinkTicketIn struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	NodeID   string    `json:"node_id" jsonschema:"The frame or component (n_…) to link"`
	Ticket   string    `json:"ticket" jsonschema:"The ticket key, e.g. PAY-214"`
	Unlink   bool      `json:"unlink,omitempty" jsonschema:"Remove the link instead"`
	Why      string    `json:"why,omitempty" jsonschema:"Why the frame belongs with the ticket, in a few words"`
}

// designLinkTicket is design_link_ticket: a link_ticket (or unlink_ticket) op
// through the suggestion route, so it follows the person's review rules. Runs
// on a linked ticket receive the frame's PNG and Inspect output.
func designLinkTicket(ctx context.Context, d Deps, c agents.Caller, in DesignLinkTicketIn) (any, error) {
	s, _, v, err := designTarget(ctx, d, c, "design_link_ticket", in.CanvasID)
	if err != nil {
		return nil, err
	}
	if _, err := d.Q.GetTicketByKey(ctx, store.GetTicketByKeyParams{WorkspaceID: c.Agent.WorkspaceID, Key: in.Ticket}); errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Invalid("ticket", "no ticket %s in this workspace", in.Ticket)
	} else if err != nil {
		return nil, err
	}
	op, summary := "link_ticket", "Link a frame to "+in.Ticket
	if in.Unlink {
		op, summary = "unlink_ticket", "Unlink a frame from "+in.Ticket
	}
	why := in.Why
	if why == "" {
		why = summary
	}
	return propose(ctx, d, c, s, in.CanvasID, v.Seq, summary, []design.Op{{"op": op, "node": in.NodeID, "ticket": in.Ticket, "why": why}})
}
