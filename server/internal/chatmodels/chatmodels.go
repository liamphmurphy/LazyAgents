// Package chatmodels is the catalog of harnesses and models a chat can use.
// It sits below both httpapi (GET /chat-models) and runs (selection
// validation) and imports neither.
package chatmodels

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/harness"
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

// claudeModels are the definition aliases harness.AnthropicModel maps to API ids.
var claudeModels = []Model{
	{ID: "fable-5.1", Label: "Fable 5.1"},
	{ID: "opus-5.5", Label: "Opus 5.5"},
	{ID: "sonnet-5.5", Label: "Sonnet 5.5", Default: true},
	{ID: "haiku-5.5", Label: "Haiku 5.5"},
}

const codexDefault = "gpt-5-codex"

// models returns a harness's models in display order. Codex models are the
// ones harness.Prices can cost.
func models(h string) []Model {
	switch h {
	case agentdef.HarnessClaudeCode:
		return append([]Model(nil), claudeModels...)
	case agentdef.HarnessCodex:
		ids := make([]string, 0, len(harness.Prices))
		for id := range harness.Prices {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		out := make([]Model, len(ids))
		for i, id := range ids {
			out[i] = Model{ID: id, Label: id, Default: id == codexDefault}
		}
		return out
	}
	return nil
}

var labels = map[string]string{agentdef.HarnessClaudeCode: "Claude Code", agentdef.HarnessCodex: "Codex"}

// Catalog lists and validates chat selections.
type Catalog struct {
	// RequireKey mirrors the runner: a harness without its API-key secret
	// can't run (docker backend), so it is reported unavailable.
	RequireKey bool
}

// List returns the harnesses in display order. Availability reads only
// whether the key secret exists, never its value.
func (c Catalog) List(ctx context.Context, q *store.Queries, ws uuid.UUID) ([]Harness, error) {
	out := make([]Harness, 0, len(agentdef.Harnesses))
	for _, name := range agentdef.Harnesses {
		h := Harness{Harness: name, Label: labels[name], Available: true, Models: models(name)}
		if c.RequireKey {
			ok, err := q.SecretExists(ctx, store.SecretExistsParams{WorkspaceID: ws, Name: KeySecret[name]})
			if err != nil {
				return nil, err
			}
			if !ok {
				h.Available, h.UnavailableReason = false, "no "+KeySecret[name]+" secret"
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
	if reqHarness != "" && reqHarness != currentHarness && !h.Available {
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
