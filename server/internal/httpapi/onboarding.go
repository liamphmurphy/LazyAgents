package httpapi

import (
	"context"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// The first-run Welcome Wizard: whether it has been finished.

// harnessConnected reports whether chats have a harness to run on. Runs that
// use the host's CLI logins (the local backend) always do; otherwise one of
// the harness key secrets has to exist. Discovery is too slow for every
// profile read, so this only looks for the secret.
func (s *Server) harnessConnected(ctx context.Context, ws uuid.UUID) (bool, error) {
	if s.Runs == nil || !s.Runs.RequireHarnessKey {
		return true, nil
	}
	for _, name := range chatmodels.KeySecret {
		ok, err := s.Q.SecretExists(ctx, store.SecretExistsParams{WorkspaceID: ws, Name: name})
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func (s *Server) CompleteOnboarding(ctx context.Context, _ api.CompleteOnboardingRequestObject) (api.CompleteOnboardingResponseObject, error) {
	u, _, ws := me(ctx)
	out, err := s.Q.CompleteOnboarding(ctx, store.CompleteOnboardingParams{ID: u.ID, WorkspaceID: ws})
	if err != nil {
		return nil, err
	}
	p, err := s.toMe(ctx, out)
	return api.CompleteOnboarding200JSONResponse(p), err
}
