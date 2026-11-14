// Package chatmodels is the catalog of harnesses and models a chat can use.
// It sits below both httpapi (GET /chat-models) and runs (selection
// validation) and imports neither.
package chatmodels

import (
	"context"
	"errors"
	"slices"
	"sync"

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
	// Antigravity authenticates through `agy acp --login` inside a runner
	// container, stored as a native login bundle in harness:google.
	agentdef.HarnessAntigravity: "harness:google",
}

type Model struct {
	ID      string
	Label   string
	Default bool
	// Efforts are the reasoning efforts the model offers (agentdef.Efforts
	// order); empty when it has no effort control. DefaultEffort is the
	// model's own default, when the harness reports one.
	Efforts       []string
	DefaultEffort string
}

// EffortDefault in a request clears a chat's effort back to automatic (AutoEffort).
const EffortDefault = "default"

// fallbackEfforts are what Claude Code and Codex models offer when discovery
// can't say (provider APIs don't report effort levels; the local CLIs do).
// Antigravity models get their supported efforts directly from ParseAgyModels.
var fallbackEfforts = map[string][]string{
	agentdef.HarnessClaudeCode: agentdef.Efforts,
	agentdef.HarnessCodex:      agentdef.Efforts,
}

// knownEfforts keeps the levels Vesper can pass to a harness, in
// agentdef.Efforts order. It drops others, such as Codex's "ultra", which
// delegates automatically and so doesn't fit a chat.
func knownEfforts(levels []string) []string {
	out := []string{}
	for _, e := range agentdef.Efforts {
		if slices.Contains(levels, e) {
			out = append(out, e)
		}
	}
	return out
}

type Harness struct {
	Harness           string
	Label             string
	Available         bool
	Checking          bool
	UnavailableReason string
	Models            []Model
}

var labels = map[string]string{agentdef.HarnessClaudeCode: "Claude Code", agentdef.HarnessCodex: "Codex", agentdef.HarnessAntigravity: "Antigravity"}

// chatHarnesses are the harnesses a chat can use, in picker order.
var chatHarnesses = []string{agentdef.HarnessClaudeCode, agentdef.HarnessCodex, agentdef.HarnessAntigravity}

// Catalog lists and validates chat selections.
type Catalog struct {
	// RequireKey mirrors the runner: a harness without its API-key secret
	// can't run (docker backend), so it is reported unavailable.
	RequireKey bool
	Secrets    *secrets.Service
	Discover   func(context.Context, string, string) ([]Model, error)
	// PlanModels lists the models of a saved plan login where the API has no
	// CLI (the Docker backend), by asking a runner. Its errors are shown to
	// the person as the reason the harness is unavailable, so they must be
	// public-safe. With wait it may block for a runner's answer; without,
	// it only queues the request. Discover, when set, still takes precedence.
	PlanModels func(ctx context.Context, ws uuid.UUID, harness string, wait bool) ([]Model, error)
}

// List returns live harness models in display order. Credentials stay
// server-side; unavailable providers have an empty list and a public reason.
// It never waits on a runner, so it is safe inside a transaction: a plan
// login whose models aren't known yet is reported as still being checked.
func (c Catalog) List(ctx context.Context, q *store.Queries, ws uuid.UUID) ([]Harness, error) {
	return c.list(ctx, q, ws, false)
}

// ListWaiting is List for callers outside any transaction: a plan login with
// no models yet waits briefly for a runner to list them.
func (c Catalog) ListWaiting(ctx context.Context, q *store.Queries, ws uuid.UUID) ([]Harness, error) {
	return c.list(ctx, q, ws, true)
}

func (c Catalog) list(ctx context.Context, q *store.Queries, ws uuid.UUID, wait bool) ([]Harness, error) {
	out := make([]Harness, len(chatHarnesses))
	errs := make([]error, len(chatHarnesses))
	if wait {
		// Concurrently, so the worst case is one wait rather than one per harness.
		var wg sync.WaitGroup
		for i, name := range chatHarnesses {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out[i], errs[i] = c.harness(ctx, q, ws, name, true)
			}()
		}
		wg.Wait()
	} else {
		for i, name := range chatHarnesses {
			out[i], errs[i] = c.harness(ctx, q, ws, name, false)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	rememberOffered(ws, out)
	return out, nil
}

// Check runs discovery for one harness, waiting like ListWaiting. It is the
// Welcome Wizard's "Verify": false for a harness chats don't support.
func (c Catalog) Check(ctx context.Context, q *store.Queries, ws uuid.UUID, name string) (Harness, bool, error) {
	if !slices.Contains(chatHarnesses, name) {
		return Harness{}, false, nil
	}
	h, err := c.harness(CurrentLoginOnly(ctx), q, ws, name, true)
	if err == nil {
		rememberOffered(ws, []Harness{h})
	}
	return h, true, err
}

// CheckKey lists the models a candidate API key (not a plan login) would
// offer, before anything stores it. The error is public-safe.
func (c Catalog) CheckKey(ctx context.Context, name, key string) ([]Model, error) {
	discover := c.Discover
	if discover == nil {
		discover = Discover
	}
	models, err := discover(ctx, name, key)
	if err != nil {
		return nil, errors.New("model discovery failed; check provider credentials and connectivity")
	}
	return models, nil
}

// PlanLogin reports whether a harness key value is a subscription login
// (which only a runner can list) rather than an API key.
func PlanLogin(harness, key string) bool { return subscriptionLogin(harness, key) }

func (c Catalog) harness(ctx context.Context, q *store.Queries, ws uuid.UUID, name string, wait bool) (Harness, error) {
	h := Harness{Harness: name, Label: labels[name], Available: true, Models: []Model{}}
	if c.RequireKey {
		ok, err := q.SecretExists(ctx, store.SecretExistsParams{WorkspaceID: ws, Name: KeySecret[name]})
		if err != nil {
			return h, err
		}
		if !ok {
			h.Available, h.UnavailableReason = false, "no "+KeySecret[name]+" secret"
		}
	}
	if !h.Available {
		return h, nil
	}
	key := ""
	if c.RequireKey && c.Secrets != nil {
		var err error
		key, err = c.Secrets.Reveal(ctx, ws, nil, KeySecret[name])
		if err != nil {
			h.Available = false
			h.UnavailableReason = "model discovery requires a readable " + KeySecret[name] + " secret"
			return h, nil
		}
	}
	// Provider model APIs don't take subscription logins, so those
	// list models through the CLI, like local mode.
	viaCLI := !c.RequireKey || subscriptionLogin(name, key)
	discover := c.Discover
	queued := false
	if discover == nil {
		discover = Discover
		if viaCLI {
			discover = DiscoverLocal
			if c.RequireKey && c.PlanModels != nil {
				queued = true
				discover = func(ctx context.Context, name, _ string) ([]Model, error) {
					return c.PlanModels(ctx, ws, name, wait)
				}
			}
		}
	}
	discovered, err := discover(ctx, name, key)
	if err != nil {
		h.Available = false
		h.Checking = errors.Is(err, errStillChecking)
		h.UnavailableReason = "model discovery failed; check provider credentials and connectivity"
		if queued {
			h.UnavailableReason = err.Error()
		} else if viaCLI {
			h.UnavailableReason = "Could not read models from the local CLI. Check that it is installed and signed in on the API host."
		}
	} else if len(discovered) == 0 {
		h.Available = false
		h.UnavailableReason = "no chat models available"
	} else {
		h.Models = discovered
	}
	return h, nil
}

// Selection is a chat's harness, model and reasoning effort ("" is
// automatic: AutoEffort picks one each turn). AutoModel routes the model per
// turn (AutoModel); Model is then the one the next turn starts on.
type Selection struct {
	Harness, Model, Effort string
	AutoModel              bool
}

// Resolve validates a requested selection against the catalog. Empty fields
// fall back to current: an empty harness keeps the current one, and an empty
// model keeps the current one unless the harness changes, in which case it is
// the new harness's default. An empty effort keeps the current one when the
// selected model offers it (otherwise the harness default), and
// EffortDefault clears it. ModelAuto turns automatic routing on, starting from
// the harness's current model (its default when the harness changes); any
// other harness or model turns it off. Errors are 422s with field harness,
// model or effort.
func (c Catalog) Resolve(ctx context.Context, q *store.Queries, ws uuid.UUID, cur, req Selection) (Selection, error) {
	if req.Harness == "" && req.Model == "" && req.Effort == "" {
		return cur, nil
	}
	list, err := c.List(ctx, q, ws)
	if err != nil {
		return Selection{}, err
	}
	auto := cur.AutoModel
	reqModel := req.Model
	switch {
	case reqModel == ModelAuto:
		auto, reqModel = true, ""
	case reqModel != "" || req.Harness != "":
		auto = false
	}
	h, m, err := resolveModel(list, cur.Harness, cur.Model, req.Harness, reqModel)
	if err != nil {
		return Selection{}, err
	}
	out := Selection{Harness: h.Harness, Model: m, Effort: cur.Effort, AutoModel: auto}
	// A model the catalog doesn't list (the definition's alias) may take any
	// effort the harness supports.
	efforts := fallbackEfforts[h.Harness]
	for _, x := range h.Models {
		if x.ID == m {
			efforts = x.Efforts
		}
	}
	switch req.Effort {
	case "":
		if !slices.Contains(efforts, out.Effort) {
			out.Effort = ""
		}
	case EffortDefault:
		out.Effort = ""
	default:
		if !slices.Contains(efforts, req.Effort) {
			return Selection{}, apperr.Invalid("effort", "%s doesn't offer effort %q", m, req.Effort)
		}
		out.Effort = req.Effort
	}
	return out, nil
}

func resolveModel(list []Harness, currentHarness, currentModel, reqHarness, reqModel string) (*Harness, string, error) {
	harnessName, model := currentHarness, currentModel
	if reqHarness != "" {
		harnessName = reqHarness
	}
	var h *Harness
	for i := range list {
		if list[i].Harness == harnessName {
			h = &list[i]
		}
	}
	if h == nil {
		return nil, "", apperr.Invalid("harness", "unknown harness %q", reqHarness)
	}
	if !h.Available {
		return nil, "", apperr.Invalid("harness", "%s is unavailable: %s", h.Label, h.UnavailableReason)
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
	// Restating the current model is the same as omitting it: the definition's
	// model (e.g. "opus-5.5") is an alias the live catalog may not list.
	if harnessName == currentHarness && (reqModel == "" || reqModel == currentModel) {
		return h, model, nil
	}
	for _, m := range h.Models {
		if m.ID == model {
			return h, model, nil
		}
	}
	return nil, "", apperr.Invalid("model", "unknown model %q for %s", model, h.Label)
}
