package runner

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// DefaultChatImageTimeout bounds one Codex image step.
const DefaultChatImageTimeout = 5 * time.Minute

// imageStepPrompt wraps a job's prompt for the one-shot Codex step.
const imageStepPrompt = "Generate exactly one image for this request with your image generation tool, then copy the final file into .lazyagents/outbox/ with cp. Don't do anything else. Request: "

// imageStepInstructions is the step's AGENTS.md: a stand-in for the chat
// persona's instructions, which are meant for the chat's own model.
const imageStepInstructions = `This is a one-shot image step inside a chat. Make the one image the request asks for with your image generation tool, copy the final file into .lazyagents/outbox/ (png, jpeg, webp or gif), and stop. Don't modify any other file, don't run other commands, and don't call Vesper tools.
`

// chatImages is what a turn's image jobs did, for finishChatTurn: whether any
// ran (so the outbox is harvested) and why the failed ones failed.
type chatImages struct {
	ran      bool
	failures []string
}

// imageStepSpec is the frozen-spec equivalent of ChatSpec for an image-only
// Codex chat: read-only plus the outbox permission profile, image generation
// on, no web, no scopes, no resumed session.
func imageStepSpec(chat agentdef.Resolved, model string) agentdef.Resolved {
	s := chat
	s.Scopes = slices.Clone(chat.Scopes)
	for i := range s.Scopes {
		s.Scopes[i].Mode = agentdef.Denied
	}
	s.Harness, s.Model = agentdef.HarnessCodex, model
	s.HarnessOptions = map[string]string{"sandbox": "read-only", "readonly": "true", agentdef.OptionImageGeneration: "true"}
	s.Setup, s.MCPServers, s.Fallbacks, s.Repository, s.Skills = []string{}, []agentdef.MCPServer{}, nil, nil, nil
	s.Instructions = imageStepInstructions
	return s
}

// runChatImageJobs makes the images a non-Codex chat turn asked for with
// generate_image, one Codex step each, after the turn's own process exited
// successfully and before its reply is stored. Failures never fail the turn:
// they are reported to finishChatTurn, which says so under the reply.
func (r *Runner) runChatImageJobs(ctx context.Context, c runCtx, turn store.RunTurn, token string) chatImages {
	var out chatImages
	jobs, err := r.q().QueuedChatImageJobs(ctx, store.QueuedChatImageJobsParams{RunID: c.run.ID, TurnID: turn.ID})
	if err != nil {
		r.Log.Warn("queued image jobs", "run", c.run.ID, "err", err)
		return out
	}
	stopped := false
	for _, job := range jobs {
		out.ran = true
		if stopped {
			// The ones after a stop never started.
			_ = r.q().FinishChatImageJob(context.WithoutCancel(ctx), store.FinishChatImageJobParams{ID: job.ID, State: "failed", Error: new("stopped")})
			out.failures = append(out.failures, "stopped")
			continue
		}
		ok, reason := r.runChatImageJob(ctx, c, turn, token, job)
		if ok {
			continue
		}
		out.failures = append(out.failures, reason)
		stopped = ctx.Err() != nil || reason == "stopped"
	}
	return out
}

// runChatImageJob runs one job. On failure it returns a short, secret-free
// reason that is safe to show the person.
func (r *Runner) runChatImageJob(ctx context.Context, c runCtx, turn store.RunTurn, token string, job store.ChatImageJob) (done bool, reason string) {
	fin := context.WithoutCancel(ctx)
	q := r.q()
	finish := func(reason string) (bool, string) {
		state, errp := "done", (*string)(nil)
		if reason != "" {
			state, errp = "failed", &reason
		}
		if err := q.FinishChatImageJob(fin, store.FinishChatImageJobParams{ID: job.ID, State: state, Error: errp}); err != nil {
			r.Log.Warn("finish image job", "job", job.ID, "err", err)
		}
		r.event(fin, c.run.ID, "tool_result", map[string]any{"id": job.ID.String(), "tool": "image_generation", "is_error": reason != ""})
		return reason == "", reason
	}
	if run, err := q.GetRun(ctx, store.GetRunParams{ID: c.run.ID, WorkspaceID: c.run.WorkspaceID}); err != nil || run.State != "running" {
		_ = q.FinishChatImageJob(fin, store.FinishChatImageJobParams{ID: job.ID, State: "failed", Error: new("stopped")})
		return false, "stopped"
	}
	if err := q.StartChatImageJob(ctx, job.ID); err != nil {
		r.Log.Warn("start image job", "job", job.ID, "err", err)
		return false, "stopped"
	}
	// The prompt stays out of the event: it is the person's text, and the run
	// log is read by more people than the chat is.
	r.event(ctx, c.run.ID, "tool_call", map[string]any{"id": job.ID.String(), "tool": "image_generation", "input": map[string]any{}})

	spec := imageStepSpec(c.spec, job.Model)
	plan, err := PlanSecrets(ctx, q, c.run.WorkspaceID, &c.project.ID, spec)
	if err != nil {
		r.Log.Warn("image step secrets", "run", c.run.ID, "err", err)
		return finish("Codex isn't set up")
	}
	plan = ChatSecretPlan(plan, spec)
	if key, _ := HarnessKey(spec); NeedsHarnessKey(r.Cfg.RequireHarnessKey, spec) && !hasSecret(plan, key.Secret) {
		return finish("Codex isn't set up")
	}
	env, err := SecretEnv(ctx, r.Secrets, c.run.WorkspaceID, &c.project.ID, plan)
	if err != nil {
		r.Log.Warn("image step secrets", "run", c.run.ID, "err", err)
		return finish("Codex isn't set up")
	}
	ScrubChatGitEnv(env, "")
	cacheDir := ""
	if backend, ok := r.Backend.(CacheBackend); ok {
		cacheDir = backend.DependencyCacheDir()
	}
	dir := childDir(c.run)
	h := harness.Codex{Prices: r.Cfg.ModelPrices}
	inv, err := h.Invocation(harness.Workspace{
		DependencyCacheDir: cacheDir, Dir: r.Backend.WorkspaceDir(wsID(c.run)), MCPURL: r.Cfg.MCPURL, Token: token, TokenEnv: tokenEnv,
	}, harness.Turn{Spec: spec, Prompt: imageStepPrompt + job.Prompt})
	if err != nil {
		r.Log.Warn("image step invocation", "run", c.run.ID, "err", err)
		return finish("Codex couldn't be started")
	}
	files := map[string][]byte{
		".lazyagents/agents-section.md": []byte(imageStepInstructions),
		// The directory the step may write to; dot files are never harvested.
		".lazyagents/outbox/.keep": {},
	}
	for k, v := range inv.Files {
		files[k] = v
	}
	for k, v := range inv.Env {
		env[k] = v
	}
	sensitive := make(map[string]string, len(env))
	for k, v := range env {
		sensitive[k] = v
	}
	env["LAZYAGENTS_RUN_ID"] = c.run.ID.String()
	env["LAZYAGENTS_TICKET"] = c.ticket.Key

	before := r.outboxNames(ctx, c)
	proc, err := r.Backend.Start(ctx, wsID(c.run), ExecSpec{
		Name: fmt.Sprintf("lazyagents-image-%s-%d-%s", c.run.ID, turn.Seq, job.ID.String()[:8]),
		Argv: append(append([]string{}, TurnWrapper...), inv.Argv...), Env: env, Dir: dir, Files: files, StateFiles: inv.StateFiles,
	})
	if err != nil {
		r.Log.Warn("start image step", "run", c.run.ID, "err", err)
		return finish("Codex couldn't be started")
	}
	timeout := r.Cfg.ChatImageTimeout
	if timeout <= 0 {
		timeout = DefaultChatImageTimeout
	}
	start := time.Now()
	// The Codex thread is the job's, never the run's resumable session: the
	// policy hook lets the step copy from that thread's directory only.
	res, _, code, waitErr := r.watchSession(ctx, c, proc, h.NewParser(spec.Model), sensitive, timeout, func(s string) {
		if err := q.SetChatImageJobSession(context.Background(), store.SetChatImageJobSessionParams{ID: job.ID, CodexSessionID: &s}); err != nil {
			r.Log.Warn("record image step session", "job", job.ID, "err", err)
		}
	})
	if res.Usage != (harness.Usage{}) {
		var source *string
		if res.Usage.CostSource != "" {
			source = &res.Usage.CostSource
		}
		if _, err := q.AddRunUsage(fin, store.AddRunUsageParams{ID: c.run.ID, ToolCalls: int32(res.Usage.ToolCalls), InputTokens: res.Usage.InputTokens,
			OutputTokens: res.Usage.OutputTokens, CostUsdMicros: int64(res.Usage.CostUSD * 1e6), CostSource: source}); err != nil {
			r.Log.Warn("record image step usage", "run", c.run.ID, "err", err)
		}
	}
	switch run, err := q.GetRun(fin, store.GetRunParams{ID: c.run.ID, WorkspaceID: c.run.WorkspaceID}); {
	case ctx.Err() != nil:
		return finish("interrupted")
	case err == nil && run.State != "running":
		return finish("stopped")
	case code != 0 && time.Since(start) >= timeout:
		return finish("timed out")
	case res.UsageLimited:
		return finish("Codex has hit its usage limit")
	case waitErr != nil || code != 0 || res.IsError:
		return finish("Codex couldn't finish the image")
	}
	if !slices.ContainsFunc(r.outboxNames(ctx, c), func(n string) bool { return !slices.Contains(before, n) }) {
		return finish("no image was produced")
	}
	return finish("")
}

// outboxNames lists the files in the chat run's outbox.
func (r *Runner) outboxNames(ctx context.Context, c runCtx) []string {
	files, _, err := r.Backend.ReadOutbox(ctx, wsID(c.run))
	if err != nil {
		return nil
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	return names
}
