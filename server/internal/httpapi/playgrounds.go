package httpapi

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/playground"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

func (s *Server) GetChatPlayground(ctx context.Context, req api.GetChatPlaygroundRequestObject) (api.GetChatPlaygroundResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	_, saved, err := s.Runs.SavedPlaygroundSpec(ctx, c)
	if err != nil {
		return nil, err
	}
	pg, err := s.Q.GetChatPlayground(ctx, store.GetChatPlaygroundParams{ChatID: c.ID, WorkspaceID: c.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return api.GetChatPlayground200JSONResponse{Playground: nullable.NewNullNullable[api.Playground](), SavedSpec: saved}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetChatPlayground200JSONResponse{Playground: nullable.NewNullableWithValue(playgroundOut(pg)), SavedSpec: saved}, nil
}

func (s *Server) ChatPlaygroundAction(ctx context.Context, req api.ChatPlaygroundActionRequestObject) (api.ChatPlaygroundActionResponseObject, error) {
	c, err := s.ownChat(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	pg, err := s.Runs.PlaygroundAction(ctx, c.WorkspaceID, c.ID, req.Action)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return api.ChatPlaygroundAction200JSONResponse(playgroundOut(pg)), nil
}

func playgroundOut(pg store.Playground) api.Playground {
	out := api.Playground{
		Id: pg.ID, ChatId: pg.ChatID, State: api.PlaygroundState(pg.State), Step: pg.Step, Branch: pg.Branch,
		Error: nullableOf(pg.Error), ErrorLog: pg.ErrorLog, StopReason: pg.StopReason,
		Port: nullableOf(pg.Port), IdleTtlSeconds: nullableOf(pg.IdleTtlS),
		StartedAt: nullableOf(pg.StartedAt), DataResetAt: nullableOf(pg.DataResetAt), LastUsedAt: nullableOf(pg.LastUsedAt),
		UpdatedAt: pg.UpdatedAt,
	}
	if pg.Action != nil {
		out.Action = nullable.NewNullableWithValue(api.PlaygroundAction(*pg.Action))
	} else {
		out.Action = nullable.NewNullNullable[api.PlaygroundAction]()
	}
	var spec playground.Spec
	_ = json.Unmarshal(pg.Spec, &spec) // validated when it was stored
	out.Openapi, out.OpenapiInferred, out.OpenapiCommit = spec.OpenAPI, spec.OpenAPIInferred, spec.OpenAPICommit
	for _, p := range spec.Processes {
		out.Processes = append(out.Processes, struct {
			Name string `json:"name"`
			Port int32  `json:"port"`
		}{p.Name, int32(p.Port)})
	}
	for _, sv := range spec.Services {
		out.Services = append(out.Services, struct {
			Image string `json:"image"`
			Name  string `json:"name"`
		}{sv.Image, sv.Name})
	}
	for _, r := range spec.Routes {
		out.Routes = append(out.Routes, struct {
			Path    string `json:"path"`
			Process string `json:"process"`
		}{r.Path, r.Process})
	}
	if out.Processes == nil {
		out.Processes = []struct {
			Name string `json:"name"`
			Port int32  `json:"port"`
		}{}
	}
	if out.Services == nil {
		out.Services = []struct {
			Image string `json:"image"`
			Name  string `json:"name"`
		}{}
	}
	if out.Routes == nil {
		out.Routes = []struct {
			Path    string `json:"path"`
			Process string `json:"process"`
		}{}
	}
	return out
}
