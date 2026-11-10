package agentapi

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/playground"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

type PlaygroundProcessIn struct {
	Name    string            `json:"name" jsonschema:"Lowercase name, e.g. web or api; other processes reach it by this name"`
	Command string            `json:"command" jsonschema:"Runs under bash -lc and keeps running, e.g. npm run dev -- -H 0.0.0.0. It must listen on 0.0.0.0, not localhost"`
	Dir     string            `json:"dir,omitempty" jsonschema:"Where the command runs, relative to the workspace root"`
	Port    int               `json:"port" jsonschema:"The port it listens on"`
	Env     map[string]string `json:"env,omitempty" jsonschema:"Environment for this process only"`
}

type PlaygroundRouteIn struct {
	Path    string `json:"path" jsonschema:"A path prefix, e.g. /api; the longest matching prefix wins"`
	Process string `json:"process" jsonschema:"The process that serves it"`
}

type StartPlaygroundIn struct {
	Processes []PlaygroundProcessIn `json:"processes" jsonschema:"The app's long-running parts, 1 to 4"`
	Setup     []string              `json:"setup,omitempty" jsonschema:"Commands run in order from the workspace root once services are up, before the processes: install, migrate, seed"`
	Routes    []PlaygroundRouteIn   `json:"routes,omitempty" jsonschema:"Which process serves which paths when there are several; everything else goes to the first process"`
	Services  []agentdef.Service    `json:"services,omitempty" jsonschema:"Services that start empty next to the app (name, image, env, healthcheck, expose). Omit to use this repository's test environment"`
	Env       map[string]string     `json:"env,omitempty" jsonschema:"Environment for setup and every process"`
	OpenAPI   string                `json:"openapi,omitempty" jsonschema:"The app's OpenAPI document, relative to the workspace root, e.g. api/openapi.yaml; the person's API tab reads it"`
}

func playgroundChat(ctx context.Context, d Deps, c agents.Caller) error {
	s, err := chatScopeFor(ctx, d, c)
	if err != nil {
		return err
	}
	if s == nil || c.RunID == nil {
		return apperr.Conflict("playgrounds only work in a chat with a person")
	}
	return nil
}

func startPlayground(ctx context.Context, d Deps, c agents.Caller, in StartPlaygroundIn) (any, error) {
	if err := playgroundChat(ctx, d, c); err != nil {
		return nil, err
	}
	spec := playground.Spec{Setup: in.Setup, Services: in.Services, Env: in.Env, OpenAPI: in.OpenAPI}
	for _, p := range in.Processes {
		spec.Processes = append(spec.Processes, playground.Process{Name: p.Name, Command: p.Command, Dir: p.Dir, Port: p.Port, Env: p.Env})
	}
	for _, r := range in.Routes {
		spec.Routes = append(spec.Routes, playground.Route{Path: r.Path, Process: r.Process})
	}
	pg, err := d.Runs.StartChatPlayground(ctx, c.Agent.WorkspaceID, *c.RunID, spec)
	if err != nil {
		return nil, err
	}
	return map[string]any{"playground_id": pg.ID, "state": pg.State,
		"next":            "A runner is starting it. Call playground_status until it's running or failed; starting can take minutes when setup installs dependencies.",
		"tell_the_person": "Once it's running, write a fenced code block tagged playground holding only " + pg.ID.String() + "; the chat shows it as a card that opens the app beside the chat. Don't link a URL: only Vesper's card opens it."}, nil
}

// maxStatusLog bounds the failure output playground_status returns.
const maxStatusLog = 6000

func playgroundStatus(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	if err := playgroundChat(ctx, d, c); err != nil {
		return nil, err
	}
	pg, err := d.Runs.PlaygroundForRun(ctx, c.Agent.WorkspaceID, *c.RunID)
	if err != nil {
		return nil, err
	}
	return playgroundView(pg), nil
}

func stopPlayground(ctx context.Context, d Deps, c agents.Caller, _ Empty) (any, error) {
	if err := playgroundChat(ctx, d, c); err != nil {
		return nil, err
	}
	pg, err := d.Runs.PlaygroundForRun(ctx, c.Agent.WorkspaceID, *c.RunID)
	if err != nil {
		return nil, err
	}
	if pg, err = d.Runs.PlaygroundAction(ctx, c.Agent.WorkspaceID, pg.ChatID, runs.PlaygroundStop); err != nil {
		return nil, err
	}
	return playgroundView(pg), nil
}

func playgroundView(pg store.Playground) map[string]any {
	out := map[string]any{"playground_id": pg.ID, "state": pg.State}
	if pg.Step != "" {
		out["step"] = pg.Step
	}
	if pg.Action != nil {
		out["pending"] = *pg.Action
	}
	if pg.Branch != "" {
		out["branch"] = pg.Branch
	}
	if pg.Error != nil {
		out["error"] = *pg.Error
		log := pg.ErrorLog
		if len(log) > maxStatusLog {
			log = log[len(log)-maxStatusLog:]
		}
		out["log"] = log
	}
	if pg.StopReason != "" {
		out["stopped"] = pg.StopReason
	}
	var spec playground.Spec
	if json.Unmarshal(pg.Spec, &spec) == nil {
		var procs []string
		for _, p := range spec.Processes {
			procs = append(procs, p.Name)
		}
		out["processes"] = strings.Join(procs, ", ")
	}
	return out
}
