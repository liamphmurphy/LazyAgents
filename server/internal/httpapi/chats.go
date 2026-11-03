package httpapi

import (
	"context"
	"net/http"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
)

// Chat endpoints (wiki: Architecture/ADR - Project chat sessions). The
// contract is fixed by LAZYAGENTS-6; these stubs answer 501 until
// LAZYAGENTS-11 implements them.

func chatsNotImplemented() (int, api.Error) {
	return http.StatusNotImplemented, api.Error{Code: api.Internal, Message: "chats are not implemented yet"}
}

func (s *Server) ListChatModels(ctx context.Context, _ api.ListChatModelsRequestObject) (api.ListChatModelsResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.ListChatModelsdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) ListChats(ctx context.Context, _ api.ListChatsRequestObject) (api.ListChatsResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.ListChatsdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) CreateChat(ctx context.Context, _ api.CreateChatRequestObject) (api.CreateChatResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.CreateChatdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) GetChat(ctx context.Context, _ api.GetChatRequestObject) (api.GetChatResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.GetChatdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) UpdateChat(ctx context.Context, _ api.UpdateChatRequestObject) (api.UpdateChatResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.UpdateChatdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) DeleteChat(ctx context.Context, _ api.DeleteChatRequestObject) (api.DeleteChatResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.DeleteChatdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) ListChatMessages(ctx context.Context, _ api.ListChatMessagesRequestObject) (api.ListChatMessagesResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.ListChatMessagesdefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) PostChatMessage(ctx context.Context, _ api.PostChatMessageRequestObject) (api.PostChatMessageResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.PostChatMessagedefaultJSONResponse{StatusCode: code, Body: body}, nil
}

func (s *Server) CancelChatTurn(ctx context.Context, _ api.CancelChatTurnRequestObject) (api.CancelChatTurnResponseObject, error) {
	code, body := chatsNotImplemented()
	return api.CancelChatTurndefaultJSONResponse{StatusCode: code, Body: body}, nil
}
