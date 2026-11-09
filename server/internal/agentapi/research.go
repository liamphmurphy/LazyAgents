package agentapi

import (
	"context"
	"fmt"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/runs"
)

type SubmitFindingsIn struct {
	Findings []runs.ResearchFinding `json:"findings" jsonschema:"New findings; they add to what you've already sent"`
}

func submitFindings(ctx context.Context, d Deps, c agents.Caller, in SubmitFindingsIn) (any, error) {
	if c.RunID == nil {
		return nil, apperr.Conflict("submit_findings only works in a research run")
	}
	n, err := d.Runs.SubmitResearchFindings(ctx, *c.RunID, in.Findings)
	if err != nil {
		return nil, err
	}
	return map[string]any{"findings_saved": n}, nil
}

func submitResearchReport(ctx context.Context, d Deps, c agents.Caller, in runs.ResearchReport) (any, error) {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, apperr.Conflict("submit_research_report only works in a chat with a person")
	}
	job, sp, err := d.Runs.SaveResearchReport(ctx, c, s.ChatID, s.ProjectKey, in)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"saved": true, "artifact_id": job.ReportArtifactID}
	switch {
	case sp.Space == "":
	case sp.Added:
		out["space_source"] = fmt.Sprintf("Added to the sources of the space %q.", sp.Space)
	default:
		out["space_source"] = fmt.Sprintf("Not added to the space %q: %s. Tell the person.", sp.Space, sp.Reason)
	}
	return out, nil
}
