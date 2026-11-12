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
