package httpapi

import (
	"context"
	"net/http"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
)

// Persona endpoints (wiki: Architecture/ADR - Assistant personas). The
// contract is fixed by LAZYAGENTS-81; these stubs answer 501 until the
// personas REST ticket implements them.

func personasNotImplemented() (int, api.Error) {
	return http.StatusNotImplemented, api.Error{Code: api.Internal, Message: "personas are not implemented yet"}
}

func (s *Server) ListPersonas(ctx context.Context, _ api.ListPersonasRequestObject) (api.ListPersonasResponseObject, error) {
	code, body := personasNotImplemented()
	return api.ListPersonasdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) CreatePersona(ctx context.Context, _ api.CreatePersonaRequestObject) (api.CreatePersonaResponseObject, error) {
	code, body := personasNotImplemented()
	return api.CreatePersonadefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) GetPersona(ctx context.Context, _ api.GetPersonaRequestObject) (api.GetPersonaResponseObject, error) {
	code, body := personasNotImplemented()
	return api.GetPersonadefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) UpdatePersona(ctx context.Context, _ api.UpdatePersonaRequestObject) (api.UpdatePersonaResponseObject, error) {
	code, body := personasNotImplemented()
	return api.UpdatePersonadefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) DeletePersona(ctx context.Context, _ api.DeletePersonaRequestObject) (api.DeletePersonaResponseObject, error) {
	code, body := personasNotImplemented()
	return api.DeletePersonadefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) SetDefaultPersona(ctx context.Context, _ api.SetDefaultPersonaRequestObject) (api.SetDefaultPersonaResponseObject, error) {
	code, body := personasNotImplemented()
	return api.SetDefaultPersonadefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) GetPersonaAvatar(ctx context.Context, _ api.GetPersonaAvatarRequestObject) (api.GetPersonaAvatarResponseObject, error) {
	code, body := personasNotImplemented()
	return api.GetPersonaAvatardefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) UploadPersonaAvatar(ctx context.Context, _ api.UploadPersonaAvatarRequestObject) (api.UploadPersonaAvatarResponseObject, error) {
	code, body := personasNotImplemented()
	return api.UploadPersonaAvatardefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) DeletePersonaAvatar(ctx context.Context, _ api.DeletePersonaAvatarRequestObject) (api.DeletePersonaAvatarResponseObject, error) {
	code, body := personasNotImplemented()
	return api.DeletePersonaAvatardefaultJSONResponse{StatusCode: code, Body: body}, nil
}
