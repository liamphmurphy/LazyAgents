package httpapi

import (
	"context"
	"encoding/json"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func (s *Server) GetRunnerEnvironment(ctx context.Context, req api.GetRunnerEnvironmentRequestObject) (api.GetRunnerEnvironmentResponseObject, error) {
	_, _, workspace := me(ctx)
	status, err := s.runnerStatus(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.RunnerEnvironments(ctx, store.RunnerEnvironmentsParams{WorkspaceID: workspace, RunID: req.Params.RunId})
	if err != nil {
		return nil, err
	}
	out := api.RunnerEnvironment{Runner: status, Snapshots: []api.EnvironmentSnapshot{}}
	for _, row := range rows {
		var payload struct {
			Backend string                `json:"backend"`
			Report  api.EnvironmentReport `json:"report"`
		}
		if json.Unmarshal(row.Payload, &payload) != nil {
			continue
		}
		report := payload.Report
		if report.Status != "ready" && report.Status != "failed" && report.Status != "unconfigured" {
			continue
		}
		if report.Tools == nil {
			report.Tools = []api.EnvironmentTool{}
		}
		if report.Caches == nil {
			report.Caches = []api.EnvironmentCache{}
		}
		if report.Packages == nil {
			report.Packages = []api.EnvironmentPackage{}
		}
		if report.GoModules == nil {
			report.GoModules = []string{}
		}
		if report.NpmProjects == nil {
			report.NpmProjects = []string{}
		}
		if report.Playwright == nil {
			report.Playwright = []string{}
		}
		out.Snapshots = append(out.Snapshots, api.EnvironmentSnapshot{
			RunId: row.RunID, RunnerId: row.RunnerID, Backend: payload.Backend, RecordedAt: row.RecordedAt,
			TicketKey: row.TicketKey, ProjectKey: row.ProjectKey, ProjectName: row.ProjectName, Report: report,
		})
	}
	return api.GetRunnerEnvironment200JSONResponse(out), nil
}
