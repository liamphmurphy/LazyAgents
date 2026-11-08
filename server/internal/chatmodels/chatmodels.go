// Package chatmodels is the catalog of harnesses and models a chat can use.
// It sits below both httpapi (GET /chat-models) and runs (selection
// validation) and imports neither.
package chatmodels

import (
	"context"
	"github.com/liamphmurphy/vesper/server/internal/secrets"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// KeySecret is the API-key secret each harness authenticates with headless.
// The runner injects the same secrets (runner.HarnessKeys).
var KeySecret = map[string]string{
	agentdef.HarnessClaudeCode: "harness:anthropic",
	agentdef.HarnessCodex:      "harness:openai",
}

type Model struct {
	ID      string
	Label   string
	Default bool
}

type Harness struct {
	Harness           string
	Label             string
	Available         bool
	UnavailableReason string
	Models            []Model
}

var labels = map[string]string{agentdef.HarnessClaudeCode: "Claude Code", agentdef.HarnessCodex: "Codex", agentdef.HarnessOpenCode: "OpenCode"}

// chatHarnesses are the harnesses a chat can use, in picker order.
var chatHarnesses = []string{agentdef.HarnessClaudeCode, agentdef.HarnessCodex, agentdef.HarnessOpenCode}

// openCodeKeySecret and openCodeDiscoverAs map each OpenCode provider to its
// API-key secret and to the harness whose provider discovery lists its models.
var (
	openCodeKeySecret  = map[string]string{"anthropic": "harness:anthropic", "openai": "harness:openai"}
	openCodeDiscoverAs = map[string]string{"anthropic": agentdef.HarnessClaudeCode, "openai": agentdef.HarnessCodex}
)

// OpenCodeUnisolated is the unavailable reason (and the runner's refusal) when
// the deployment does not attest isolated OpenCode config.
const OpenCodeUnisolated = "OpenCode needs isolated config (managed config, no ambient ~/.config/opencode), which this backend does not attest. Set RUNNER_OPENCODE_ISOLATED=true only where it holds, or pick another harness."

// Catalog lists and validates chat selections.
type Catalog struct {
	// RequireKey mirrors the runner: a harness without its API-key secret
	// can't run (docker backend), so it is reported unavailable.
	RequireKey bool
	// OpenCodeIsolated is the runner's attestation of isolated OpenCode config
	// (RUNNER_OPENCODE_ISOLATED); without it OpenCode is unavailable.
	OpenCodeIsolated bool
	Secrets          *secrets.Service
	Discover         func(context.Context, string, string) ([]Model, error)
}

// List returns live harness models in display order. Credentials stay
// server-side; unavailable providers have an empty list and a public reason.
func (c Catalog) List(ctx context.Context, q *store.Queries, ws uuid.UUID) ([]Harness, error) {
	out := make([]Harness, 0, len(chatHarnesses))
	for _, name := range chatHarnesses {
		h := Harness{Harness: name, Label: labels[name], Available: true, Models: []Model{}}
		if name == agentdef.HarnessOpenCode {
			if err := c.listOpenCode(ctx, q, ws, &h); err != nil {
				return nil, err
			}
			out = append(out, h)
			continue
		}
		if c.RequireKey {
			ok, err := q.SecretExists(ctx, store.SecretExistsParams{WorkspaceID: ws, Name: KeySecret[name]})
			if err != nil {
				return nil, err
			}
			if !ok {
				h.Available, h.UnavailableReason = false, "no "+KeySecret[name]+" secret"
			}
		}
		if h.Available {
			key := ""
			if c.RequireKey && c.Secrets != nil {
				var err error
				key, err = c.Secrets.Reveal(ctx, ws, nil, KeySecret[name])
				if err != nil {
					h.Available = false
					h.UnavailableReason = "model discovery requires a readable " + KeySecret[name] + " secret"
				}
			}
			if h.Available {
				discover := c.Discover
				if discover == nil {
					discover = Discover
					if !c.RequireKey {
						discover = DiscoverLocal
					}
				}
				discovered, err := discover(ctx, name, key)
				if err != nil {
					h.Available = false
					h.UnavailableReason = "model discovery failed; check provider credentials and connectivity"
					if !c.RequireKey {
						h.UnavailableReason = "Could not read models from the local CLI. Check that it is installed and signed in on the API host."
					}
				} else if len(discovered) == 0 {
					h.Available = false
					h.UnavailableReason = "no chat models available"
				} else {
					h.Models = discovered
				}
			}
		}
		out = append(out, h)
	}
	return out, nil
}

// Resolve validates a requested selection against the catalog. Empty fields
// fall back to current: an empty harness keeps currentHarness, and an empty
// model keeps currentModel unless the harness changes, in which case it is
// the new harness's default. Errors are 422s with field harness or model.
func (c Catalog) Resolve(ctx context.Context, q *store.Queries, ws uuid.UUID, currentHarness, currentModel, reqHarness, reqModel string) (harnessName, model string, err error) {
	harnessName, model = currentHarness, currentModel
	if reqHarness == "" && reqModel == "" {
		return harnessName, model, nil
	}
	if reqHarness != "" {
		harnessName = reqHarness
	}
	list, err := c.List(ctx, q, ws)
	if err != nil {
		return "", "", err
	}
	var h *Harness
	for i := range list {
		if list[i].Harness == harnessName {
			h = &list[i]
		}
	}
	if h == nil {
		return "", "", apperr.Invalid("harness", "unknown harness %q", reqHarness)
	}
	if !h.Available {
		return "", "", apperr.Invalid("harness", "%s is unavailable: %s", h.Label, h.UnavailableReason)
	}
	switch {
	case reqModel != "":
		model = reqModel
	case harnessName != currentHarness:
		for _, m := range h.Models {
			if m.Default {
				model = m.ID
			}
		}
	}
	if reqModel == "" && harnessName == currentHarness {
		return harnessName, model, nil
	}
	for _, m := range h.Models {
		if m.ID == model {
			return harnessName, model, nil
		}
	}
	return "", "", apperr.Invalid("model", "unknown model %q for %s", model, h.Label)
}

// listOpenCode fills the OpenCode entry. It always needs an API-key secret,
// even on the local backend (there is no host login to fall back on), so
// RequireKey is ignored. Each provider with a revealable secret contributes its
// discovered models, qualified as provider/model; a provider whose secret is
// missing or whose discovery fails is skipped.
func (c Catalog) listOpenCode(ctx context.Context, q *store.Queries, ws uuid.UUID, h *Harness) error {
	if !c.OpenCodeIsolated {
		h.Available, h.UnavailableReason = false, OpenCodeUnisolated
		return nil
	}
	discover := c.Discover
	if discover == nil {
		discover = Discover
	}
	var anthropicDefault, openaiDefault, first string
	for _, provider := range agentdef.OpenCodeProviders {
		secret := openCodeKeySecret[provider]
		ok, err := q.SecretExists(ctx, store.SecretExistsParams{WorkspaceID: ws, Name: secret})
		if err != nil {
			return err
		}
		if !ok || c.Secrets == nil {
			continue
		}
		key, err := c.Secrets.Reveal(ctx, ws, nil, secret)
		if err != nil || key == "" {
			continue
		}
		discovered, err := discover(ctx, openCodeDiscoverAs[provider], key)
		if err != nil {
			continue
		}
		for _, m := range discovered {
			id := provider + "/" + m.ID
			if _, err := agentdef.OpenCodeProvider(id); err != nil {
				continue
			}
			if first == "" {
				first = id
			}
			if m.Default && provider == "anthropic" && anthropicDefault == "" {
				anthropicDefault = id
			}
			if m.Default && provider == "openai" && openaiDefault == "" {
				openaiDefault = id
			}
			h.Models = append(h.Models, Model{ID: id, Label: m.Label})
		}
	}
	if len(h.Models) == 0 {
		h.Available = false
		h.UnavailableReason = "OpenCode needs a usable harness:anthropic or harness:openai secret (and a reachable provider) to list models"
		return nil
	}
	def := anthropicDefault
	if def == "" {
		def = openaiDefault
	}
	if def == "" {
		def = first
	}
	for i := range h.Models {
		h.Models[i].Default = h.Models[i].ID == def
	}
	return nil
}
