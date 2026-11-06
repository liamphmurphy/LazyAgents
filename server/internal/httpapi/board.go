package httpapi

import (
	"context"
	"net/http"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
)

// Board and epic-criteria endpoints (wiki: Architecture/ADR - Kanban board,
// estimates and epic criteria). LAZYAGENTS-29 fixes the contract; these stubs
// answer 501 until the backend ticket implements them.

func boardNotImplemented() (int, api.Error) {
	return http.StatusNotImplemented, api.Error{Code: api.Internal, Message: "the board and epic criteria are not implemented yet"}
}

func (s *Server) GetProjectBoard(ctx context.Context, _ api.GetProjectBoardRequestObject) (api.GetProjectBoardResponseObject, error) {
	code, body := boardNotImplemented()
	return api.GetProjectBoarddefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) ListEpicCriteria(ctx context.Context, _ api.ListEpicCriteriaRequestObject) (api.ListEpicCriteriaResponseObject, error) {
	code, body := boardNotImplemented()
	return api.ListEpicCriteriadefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) CreateEpicCriterion(ctx context.Context, _ api.CreateEpicCriterionRequestObject) (api.CreateEpicCriterionResponseObject, error) {
	code, body := boardNotImplemented()
	return api.CreateEpicCriteriondefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) UpdateEpicCriterion(ctx context.Context, _ api.UpdateEpicCriterionRequestObject) (api.UpdateEpicCriterionResponseObject, error) {
	code, body := boardNotImplemented()
	return api.UpdateEpicCriteriondefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) DeleteEpicCriterion(ctx context.Context, _ api.DeleteEpicCriterionRequestObject) (api.DeleteEpicCriterionResponseObject, error) {
	code, body := boardNotImplemented()
	return api.DeleteEpicCriteriondefaultJSONResponse{StatusCode: code, Body: body}, nil
}
