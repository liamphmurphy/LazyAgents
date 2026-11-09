package httpapi

import (
	"context"
	"encoding/json"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func researchTaskState(runState string) api.ResearchTaskState {
	switch runState {
	case "queued", "starting":
		return api.ResearchTaskStateQueued
	case "completed":
		return api.ResearchTaskStateDone
	case "failed":
		return api.ResearchTaskStateFailed
	case "cancelled":
		return api.ResearchTaskStateStopped
	}
	return api.ResearchTaskStateRunning
}

func (s *Server) toResearchJob(ctx context.Context, j store.ResearchJob) (api.ResearchJob, error) {
	out := api.ResearchJob{Id: j.ID, MessageId: j.MessageID, Title: j.Title, State: api.ResearchJobState(j.State), MaxSources: int(j.MaxSources), MaxMinutes: int(j.MaxMinutes),
		ReportLength: api.ResearchReportLength(j.ReportLength), ReportTone: api.ResearchReportTone(j.ReportTone),
		ArtifactId: j.ReportArtifactID, CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt, Tasks: []api.ResearchTask{}}
	if j.StopReason != "" {
		out.StopReason = &j.StopReason
	}
	if j.Error != "" {
		out.Error = &j.Error
	}
	if j.MaxCostMicros != nil {
		c := float32(*j.MaxCostMicros) / 1e6
		out.MaxCostUsd = &c
	}
	tasks, err := s.Q.ListResearchTasks(ctx, j.ID)
	if err != nil {
		return out, err
	}
	var spent int64
	for _, t := range tasks {
		var found []json.RawMessage
		_ = json.Unmarshal(t.Findings, &found)
		task := api.ResearchTask{Label: t.Label, Question: t.Question, State: researchTaskState(t.RunState), SourcesRead: int(t.SourcesRead), Findings: len(found)}
		if t.Reading != "" {
			task.Reading = &t.Reading
		}
		out.Tasks = append(out.Tasks, task)
		out.SourcesRead += int(t.SourcesRead)
		spent += t.CostUsdMicros
	}
	out.SpentUsd = float32(spent) / 1e6
	if len(j.Report) > 0 {
		var r api.ResearchReport
		if err := json.Unmarshal(j.Report, &r); err != nil {
			return out, err
		}
		if r.Disagreements == nil {
			empty := []struct {
				Detail string `json:"detail"`
				Topic  string `json:"topic"`
			}{}
			r.Disagreements = empty
		}
		out.Report = &r
	}
	return out, nil
}

func (s *Server) ListChatResearch(ctx context.Context, req api.ListChatResearchRequestObject) (api.ListChatResearchResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListResearchJobs(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	out := api.ResearchJobs{Jobs: make([]api.ResearchJob, 0, len(rows))}
	for _, r := range rows {
		j, err := s.toResearchJob(ctx, r)
		if err != nil {
			return nil, err
		}
		out.Jobs = append(out.Jobs, j)
	}
	return api.ListChatResearch200JSONResponse(out), nil
}

func (s *Server) StartChatResearch(ctx context.Context, req api.StartChatResearchRequestObject) (api.StartChatResearchResponseObject, error) {
	u, _, ws := me(ctx)
	in := runs.ResearchInput{MessageID: req.Body.MessageId, Title: req.Body.Title, MaxSources: req.Body.MaxSources, MaxMinutes: req.Body.MaxMinutes}
	if req.Body.ReportLength != nil {
		in.Length = string(*req.Body.ReportLength)
	}
	if req.Body.ReportTone != nil {
		in.Tone = string(*req.Body.ReportTone)
	}
	if req.Body.MaxCostUsd != nil {
		c := float64(*req.Body.MaxCostUsd)
		in.MaxCostUSD = &c
	}
	for _, q := range req.Body.Questions {
		in.Questions = append(in.Questions, runs.ResearchQuestion{Label: q.Label, Question: q.Question})
	}
	job, err := s.Runs.StartResearch(ctx, ws, u.ID, req.Id, in)
	if err != nil {
		return nil, err
	}
	out, err := s.toResearchJob(ctx, job)
	return api.StartChatResearch201JSONResponse(out), err
}

func (s *Server) ControlChatResearch(ctx context.Context, req api.ControlChatResearchRequestObject) (api.ControlChatResearchResponseObject, error) {
	u, _, ws := me(ctx)
	var job store.ResearchJob
	var err error
	if string(req.Action) == "finish" {
		job, err = s.Runs.FinishResearch(ctx, ws, u.ID, req.Id, req.JobId)
	} else {
		job, err = s.Runs.StopResearch(ctx, ws, u.ID, req.Id, req.JobId)
	}
	if err != nil {
		return nil, err
	}
	out, err := s.toResearchJob(ctx, job)
	return api.ControlChatResearch200JSONResponse(out), err
}
