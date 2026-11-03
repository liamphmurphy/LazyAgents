// Package chatmodels is the catalog of harnesses and models a chat can use.
// It sits below both httpapi (GET /chat-models) and runs (selection
// validation) and imports neither.
package chatmodels

import (
	"context"
	"github.com/liamphmurphy/lazyagents/server/internal/secrets"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
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

var labels = map[string]string{agentdef.HarnessClaudeCode: "Claude Code", agentdef.HarnessCodex: "Codex"}

// chatHarnesses are the harnesses a chat can use. OpenCode runs tickets and
// planning only; chat segments, discovery and the picker follow in
// LAZYAGENTS-23/24 (docs/contracts/opencode2/ADR.md), so it stays out.
var chatHarnesses = []string{agentdef.HarnessClaudeCode, agentdef.HarnessCodex}

// Catalog lists and validates chat selections.
type Catalog struct {
	// RequireKey mirrors the runner: a harness without its API-key secret
	// can't run (docker backend), so it is reported unavailable.
	RequireKey bool
	Secrets    *secrets.Service
	Discover   func(context.Context, string, string) ([]Model, error)
}

// List returns live harness models in display order. Credentials stay
// server-side; unavailable providers have an empty list and a public reason.
func (c Catalog) List(ctx context.Context, q *store.Queries, ws uuid.UUID) ([]Harness, error) {
	out := make([]Harness, 0, len(chatHarnesses))
	for _, name := range chatHarnesses {
		h := Harness{Harness: name, Label: labels[name], Available: true, Models: []Model{}}
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
