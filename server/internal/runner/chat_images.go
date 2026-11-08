package runner

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/runs"
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
// ran, so the outbox is harvested. What each job made or why it failed is on
// its chat_image_jobs row.
type chatImages struct {
	ran bool
}

// Image job failure kinds (chat_image_jobs.failure_kind).
const (
	imageFailModeration = "moderation"
	imageFailError      = "error"
)

// maxImageReasonRunes bounds a failure reason shown to the person and the assistant.
const maxImageReasonRunes = 300

// moderationRefusal matches what Codex says (as an error or as its reply)
// when an image request was refused by moderation or a safety policy.
var moderationRefusal = regexp.MustCompile(`(?i)\bmoderat|content[ _-]?polic|safety[ _-]?(system|polic|guideline|filter|reason|rule)|polic(y|ies)[ _-]violation|violat\w*\s+([\w'’]+\s+){0,3}(polic|guideline|rule|terms)|against\s+([\w'’]+\s+){0,3}(polic|guideline|rule)|\brefus|can(no|'|’)t (help with|assist with|create|generate|make) (that|this)`)

// classifyImageFailure reports whether a failed or empty Codex step was a
// moderation refusal, with a short, single-line detail from what Codex said
// (secrets redacted, clipped). Its error is checked before its reply.
func classifyImageFailure(res harness.Result, sensitive map[string]string) (moderation bool, detail string) {
	for _, s := range []string{res.Error, res.Final} {
		if moderationRefusal.MatchString(s) {
			return true, clipReason(redact(s, sensitive))
		}
	}
	return false, ""
}

// clipReason makes s one line of at most maxImageReasonRunes runes.
func clipReason(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxImageReasonRunes {
		s = strings.TrimSpace(string(r[:maxImageReasonRunes-1])) + "…"
	}
	return s
}

// imageFailureMessage is the system message the person sees for a failed job.
func imageFailureMessage(kind, reason string) string {
	body := "Couldn't create the image: " + reason
	if kind == imageFailModeration {
		body = "The image request was " + reason
	}
	if !strings.HasSuffix(body, ".") && !strings.HasSuffix(body, "!") && !strings.HasSuffix(body, "?") && !strings.HasSuffix(body, "…") {
		body += "."
	}
	return body
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
// finishChatTurn says so under the reply and in the follow-up turn. Jobs a
// crashed attempt of this turn already finished count as having run.
func (r *Runner) runChatImageJobs(ctx context.Context, c runCtx, turn store.RunTurn, token string) chatImages {
	var out chatImages
	jobs, err := r.q().ChatImageJobsForTurn(ctx, store.ChatImageJobsForTurnParams{RunID: c.run.ID, TurnID: turn.ID})
	if err != nil {
		r.Log.Warn("image jobs", "run", c.run.ID, "err", err)
		return out
	}
	stopped := false
	for _, job := range jobs {
		out.ran = true
		if job.State != "queued" {
			continue
		}
		if stopped {
			// The ones after a stop never started.
			_ = r.q().FinishChatImageJob(context.WithoutCancel(ctx), store.FinishChatImageJobParams{ID: job.ID, State: "failed", Error: new("stopped"), FailureKind: new(imageFailError), OutboxFiles: []string{}})
			continue
		}
		ok, reason := r.runChatImageJob(ctx, c, turn, token, job)
		stopped = !ok && (ctx.Err() != nil || reason == "stopped")
	}
	return out
}

// runChatImageJob runs one job and records how it went: the outbox files it
// produced, or a short, secret-free reason that is safe to show the person
// with its kind (a moderation refusal or any other error).
func (r *Runner) runChatImageJob(ctx context.Context, c runCtx, turn store.RunTurn, token string, job store.ChatImageJob) (done bool, reason string) {
	fin := context.WithoutCancel(ctx)
	q := r.q()
	produced := []string{}
	record := func(kind, reason string) (bool, string) {
		state, errp, kindp := "done", (*string)(nil), (*string)(nil)
		if reason != "" {
			state, errp, kindp = "failed", &reason, &kind
		}
		if err := q.FinishChatImageJob(fin, store.FinishChatImageJobParams{ID: job.ID, State: state, Error: errp, FailureKind: kindp, OutboxFiles: produced}); err != nil {
			r.Log.Warn("finish image job", "job", job.ID, "err", err)
		}
		payload := map[string]any{"id": job.ID.String(), "tool": "image_generation", "is_error": reason != ""}
		if reason != "" {
			payload["failure_kind"] = kind
		}
		r.event(fin, c.run.ID, "tool_result", payload)
		return reason == "", reason
	}
	finish := func(reason string) (bool, string) { return record(imageFailError, reason) }
	if run, err := q.GetRun(ctx, store.GetRunParams{ID: c.run.ID, WorkspaceID: c.run.WorkspaceID}); err != nil || run.State != "running" {
		_ = q.FinishChatImageJob(fin, store.FinishChatImageJobParams{ID: job.ID, State: "failed", Error: new("stopped"), FailureKind: new(imageFailError), OutboxFiles: []string{}})
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
	}
	failed := waitErr != nil || code != 0 || res.IsError
	if !failed {
		for _, n := range r.outboxNames(ctx, c) {
			if !slices.Contains(before, n) {
				produced = append(produced, n)
			}
		}
	}
	if failed || len(produced) == 0 {
		// A refusal is told apart from other failures: the person learns the
		// request itself was the problem, and how to change it.
		if moderated, detail := classifyImageFailure(res, sensitive); moderated {
			if detail == "" {
				return record(imageFailModeration, "rejected by moderation")
			}
			return record(imageFailModeration, clipReason("rejected by moderation: "+detail))
		}
		if failed {
			return finish("Codex couldn't finish the image")
		}
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

// imageFollowupPrompt is an image_followup turn's input message: Vesper's,
// never shown as the person's. imageFollowup appends each job's result.
const imageFollowupPrompt = "(Vesper: this is an automatic follow-up, not a message from the person.) " +
	"The images you asked for with generate_image in your last reply have been processed. Here is what came of each request:\n"

// imageFollowupSteps tells the assistant how to react; it ends the prompt.
const imageFollowupSteps = `
Now reply to the person:
- Open each image file listed above with your file-reading tool (Claude Code's Read shows images) and look at it before you reply.
- Then send a short, in-character reaction grounded in the conversation so far. If an image doesn't match what was asked, say so honestly.
- For each failed request, tell the person plainly that the image wasn't made and why, and suggest how to adjust the request (for a moderation refusal, what to change so it's acceptable). Never describe a failed image as if it exists.
- Don't call generate_image again unless the person asks for another image in a new message.
- The images are already shown to the person under your last reply; don't paste file paths or image data.
`

// imageFollowup delivers what an image_followup turn reports: each image the
// jobs made, under .lazyagents/attachments/generated/turn-<seq>/ (the seq of
// the turn that asked for it), with the prompt that produced it, and each
// failure with its kind and reason. Paths come from the turn, position and
// sanitized file name, like chatTurnAttachments.
func (r *Runner) imageFollowup(ctx context.Context, q *store.Queries, turn store.RunTurn) (files []ContextFile, prompt string, err error) {
	jobs, err := q.ChatImageJobsForFollowup(ctx, &turn.ID)
	if err != nil {
		return nil, "", err
	}
	var b strings.Builder
	if len(jobs) == 0 {
		b.WriteString("- (no results were recorded)\n")
	}
	var total int64
	for i, j := range jobs {
		fmt.Fprintf(&b, "\n%d. Prompt: %q\n", i+1, j.Prompt)
		switch {
		case j.State == "failed":
			label, reason := "Failed (error)", "unknown error"
			if j.FailureKind != nil && *j.FailureKind == imageFailModeration {
				label = "Failed (refused by moderation)"
			}
			if j.Error != nil {
				reason = *j.Error
			}
			fmt.Fprintf(&b, "   %s: %s. No image exists for this request.\n", label, reason)
		case j.AttachmentBlobKey == nil || j.AttachmentFilename == nil || j.AttachmentContentType == nil:
			b.WriteString("   Made, but it couldn't be stored, so the person can't see it. Tell them it was lost.\n")
		default:
			name := runs.ChatAttachmentDiskName(runs.ChatAttachmentName(*j.AttachmentFilename))
			if name == "" || strings.HasPrefix(name, ".") {
				name = "image" + name
			}
			p := fmt.Sprintf(".lazyagents/attachments/generated/turn-%d/%d-%s", j.TurnSeq, i+1, name)
			var body []byte
			if r.Attachments != nil {
				if body, err = r.readAttachment(ctx, *j.AttachmentBlobKey); err != nil && !errors.Is(err, blob.ErrNotFound) {
					return nil, "", err
				}
			}
			total += int64(len(body))
			if len(body) == 0 || total > maxChatAttachmentTurnBytes {
				b.WriteString("   Made and shown to the person, but the file isn't available to you: don't describe details you can't see.\n")
				continue
			}
			files = append(files, ContextFile{Path: p, Body: body})
			fmt.Fprintf(&b, "   Made: `%s` (%s). Open it to see it.\n", p, *j.AttachmentContentType)
		}
	}
	b.WriteString(imageFollowupSteps)
	return files, b.String(), nil
}
