package httpapi

import (
	"context"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Server) GetHighlights(ctx context.Context, req api.GetHighlightsRequestObject) (api.GetHighlightsResponseObject, error) {
	u, _, ws := me(ctx)
	p := req.Params
	if p.To.Time.Before(p.From.Time) || p.To.Time.Sub(p.From.Time) >= costMaxDays*24*time.Hour {
		return nil, apperr.Invalid("to", "choose a range of 1 to 366 days")
	}
	project := p.Project
	if project != nil && *project == "" {
		project = nil
	}
	rows, err := s.Q.HighlightsBuckets(ctx, store.HighlightsBucketsParams{
		WorkspaceID: ws, UserID: u.ID, FromTime: p.From.Time, UntilTime: p.To.Time.AddDate(0, 0, 1), ProjectKey: project,
	})
	if err != nil {
		return nil, err
	}
	out := api.GetHighlights200JSONResponse{Buckets: make([]api.HighlightsBucket, 0, len(rows))}
	for _, r := range rows {
		out.Buckets = append(out.Buckets, api.HighlightsBucket{
			PersonaId: r.PersonaID, Persona: r.Persona, Harness: r.Harness, Model: r.Model,
			Effort: r.Effort, EffortLevel: r.EffortLevel, Reasons: r.Reasons,
			Turns: r.Turns, DurationMs: r.DurationMs, TimedTurns: r.TimedTurns,
		})
	}
	return out, nil
}
