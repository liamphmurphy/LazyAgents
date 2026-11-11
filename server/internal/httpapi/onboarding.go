package httpapi

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// The first-run Welcome Wizard: whether it has been finished, and the
// harness step's Verify.

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

// checkHarnessCredential checks a pasted credential has the shape the runner
// recognises for its kind (runner.PlanSecrets): a plan login is a Claude
// setup-token or a Codex auth.json, anything else is used as an API key.
// Messages never quote the value.
func checkHarnessCredential(harness string, kind api.HarnessVerifyRequestKind, v string) error {
	if v == "" {
		return apperr.Invalid("credential", "paste a credential first")
	}
	_, codexLogin := chatmodels.ParseCodexAuth(v)
	switch {
	case harness == agentdef.HarnessClaudeCode && kind == api.Subscription && !chatmodels.ClaudeOAuthToken(v):
		return apperr.Invalid("credential", "a Claude subscription token starts with sk-ant-oat; run `claude setup-token` to get one")
	case harness == agentdef.HarnessClaudeCode && kind == api.ApiKey && (chatmodels.ClaudeOAuthToken(v) || !strings.HasPrefix(v, "sk-ant-")):
		return apperr.Invalid("credential", "an Anthropic API key starts with sk-ant-")
	case harness == agentdef.HarnessCodex && kind == api.Subscription && !codexLogin:
		return apperr.Invalid("credential", "paste the whole ~/.codex/auth.json from `codex login` (a ChatGPT sign-in)")
	case harness == agentdef.HarnessCodex && kind == api.ApiKey && (codexLogin || !strings.HasPrefix(v, "sk-")):
		return apperr.Invalid("credential", "an OpenAI API key starts with sk-")
	case kind != api.Subscription && kind != api.ApiKey:
		return apperr.Invalid("kind", "must be subscription or api_key")
	}
	return nil
}

func (s *Server) VerifyHarness(ctx context.Context, req api.VerifyHarnessRequestObject) (api.VerifyHarnessResponseObject, error) {
	u, _, ws := me(ctx)
	name := string(req.Harness)
	secret, ok := chatmodels.KeySecret[name]
	if !ok {
		return nil, apperr.Invalid("harness", "unknown harness %q", name)
	}
	if b := req.Body; b != nil {
		v := strings.TrimSpace(b.Credential)
		if err := checkHarnessCredential(name, b.Kind, v); err != nil {
			return nil, err
		}
		if s.Secrets == nil {
			return nil, secrets.ErrNoKey
		}
		if _, err := s.Secrets.Set(ctx, ws, u.ID, secrets.SetInput{Name: secret, Value: v}); err != nil {
			return nil, err
		}
	}
	h, _, err := s.Runs.ChatCatalog().Check(ctx, s.Q, ws, name)
	if err != nil {
		return nil, err
	}
	out := api.VerifyHarness200JSONResponse{Ok: h.Available && len(h.Models) > 0}
	out.Models = make([]struct {
		Id    string `json:"id"`
		Label string `json:"label"`
	}, len(h.Models))
	for i, m := range h.Models {
		out.Models[i].Id, out.Models[i].Label = m.ID, m.Label
	}
	if h.Checking {
		out.Checking = &h.Checking
	}
	if !out.Ok && h.UnavailableReason != "" {
		reason := h.UnavailableReason
		out.Error = &reason
	}
	return out, nil
}
