package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/harness"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Every chat mutation takes the session lock before its run lock. The session
// serializes posts, cancellation, completion, message seq and state notifications.
func lockRunChat(ctx context.Context, q *store.Queries, run store.Run) (store.ChatSession, error) {
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if err != nil {
		return chat, err
	}
	return q.LockChatSession(ctx, store.LockChatSessionParams{ID: chat.ID, WorkspaceID: run.WorkspaceID})
}

// chatSetupScript is a chat segment's workspace setup: the coding persona's
// repository on the chat's work branch when the frozen spec grants repo:write
// and the chat has one, otherwise just the context directory (and the outbox
// image-generating chats copy their pictures to).
func chatSetupScript(ctx context.Context, q *store.Queries, c runCtx) (string, error) {
	script := "mkdir -p .lazyagents/context"
	if c.spec.HarnessOptions[agentdef.OptionImageGeneration] == "true" {
		script += " .lazyagents/outbox"
	}
	if !allowedScope(c.spec, "repo:write") {
		return script, nil
	}
	chat, err := q.ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		return "", err
	}
	if chat.RepoUrl == nil || chat.WorkBranch == nil {
		return script, nil
	}
	name := "Assistant"
	if chat.PersonaID != nil {
		p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *chat.PersonaID, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if err == nil {
			name = p.Name
		}
	}
	return ChatRepoSetupScript(*chat.RepoUrl, *chat.WorkBranch, name+" (Vesper)", personas.Slug(name)+"@personas.lazyagents.local"), nil
}

// chatGitRepo is the repository URL a coding chat's git credentials may be
// used for: "" unless the frozen spec grants repo:write and the chat has one.
func chatGitRepo(ctx context.Context, q *store.Queries, c runCtx) (string, error) {
	if c.run.Purpose != "chat" || !allowedScope(c.spec, "repo:write") {
		return "", nil
	}
	chat, err := q.ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		return "", err
	}
	if chat.RepoUrl == nil || chat.WorkBranch == nil {
		return "", nil
	}
	return *chat.RepoUrl, nil
}

func idleChat(ctx context.Context, q *store.Queries, chat store.ChatSession, run store.Run, state string) error {
	reason := "human_reply"
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "waiting", WaitReason: &reason}); err != nil {
		return err
	}
	if _, err := q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run.ID, State: state}); err != nil {
		return err
	}
	return runs.RecomputeAgentStatus(ctx, q, run.AgentID)
}

// generatedImage is an outbox file already stored as a blob, waiting to be
// bound to the turn's assistant message.
type generatedImage struct {
	outbox   string // the file's name in the outbox
	name, ct string
	key      string
	size     int64
}

// harvestChatImages stores the images an image-generating chat copied into
// its outbox. It runs before the turn's transaction so no blob I/O happens
// under row locks; a failure is logged as a run event and never fails the turn.
func (r *Runner) harvestChatImages(ctx context.Context, c runCtx) (out []generatedImage) {
	warn := func(state string, detail any) {
		r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": state, "detail": detail})
	}
	if r.Blobs == nil || r.Cfg.AttachmentBucket == "" {
		warn("chat_images_skipped", "no attachment storage is configured")
		return nil
	}
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		r.Log.Warn("harvest chat images", "run", c.run.ID, "err", err)
		return nil
	}
	stored, err := r.q().ChatGeneratedImageBytes(ctx, store.ChatGeneratedImageBytesParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if err != nil {
		r.Log.Warn("generated image usage", "run", c.run.ID, "err", err)
		return nil
	}
	if stored >= runs.MaxGeneratedImageStorageBytes {
		warn("chat_images_skipped", "quota")
		return nil
	}
	files, skipped, err := r.Backend.ReadOutbox(ctx, c.run.ID.String())
	if err != nil {
		r.Log.Warn("read chat outbox", "run", c.run.ID, "err", err)
		warn("chat_images_failed", err.Error())
		return nil
	}
	for _, f := range files {
		ct, ok := runs.GeneratedImageType(f.Data)
		name := runs.ChatAttachmentName(f.Name)
		if !ok || name == "" || len(f.Data) == 0 {
			skipped = append(skipped, f.Name+": not a png, jpeg, webp or gif image")
			continue
		}
		if stored+int64(len(f.Data)) > runs.MaxGeneratedImageStorageBytes {
			skipped = append(skipped, f.Name+": quota")
			continue
		}
		stored += int64(len(f.Data))
		key := runs.ChatAttachmentKey(chat.WorkspaceID, chat.CreatedBy)
		if err := r.Blobs.Put(ctx, r.Cfg.AttachmentBucket, key, bytes.NewReader(f.Data), int64(len(f.Data)), ct); err != nil {
			r.Log.Warn("store generated image", "run", c.run.ID, "err", err)
			skipped = append(skipped, f.Name+": couldn't be stored")
			continue
		}
		out = append(out, generatedImage{outbox: f.Name, name: name, ct: ct, key: key, size: int64(len(f.Data))})
	}
	if len(skipped) > 0 {
		warn("chat_images_skipped", skipped)
	}
	return out
}

// dropImageBlobs removes stored images that never got a row, through the
// store they were written to. A runner crash between the Put and the turn's
// commit can still orphan blobs; a sweep for those is a follow-up.
func (r *Runner) dropImageBlobs(imgs []generatedImage) {
	if r.Blobs == nil {
		return
	}
	for _, im := range imgs {
		if err := r.Blobs.Delete(context.WithoutCancel(context.Background()), r.Cfg.AttachmentBucket, im.key); err != nil {
			r.Log.Warn("remove unbound generated image", "key", im.key, "err", err)
		}
	}
}

func (r *Runner) finishChatTurn(ctx context.Context, c runCtx, turn store.RunTurn, res harness.Result, code int, waitErr error, logKey *string, codeChanges []byte, jobs chatImages) {
	var imgs []generatedImage
	// A Codex chat copies its pictures to the outbox itself; any other chat's
	// come from the Codex steps its generate_image requests ran.
	if (c.spec.HarnessOptions[agentdef.OptionImageGeneration] == "true" || jobs.ran) && waitErr == nil && code == 0 && !res.IsError && !res.UsageLimited {
		imgs = r.harvestChatImages(ctx, c)
	}
	bound := false
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		chat, err := lockRunChat(ctx, q, c.run)
		if err != nil {
			return err
		}
		run, err := q.LockRun(ctx, c.run.ID)
		if err != nil {
			return err
		}
		usage, err := json.Marshal(res.Usage)
		if err != nil {
			return err
		}
		exit := int32(code)
		if err = q.FinishTurn(ctx, store.FinishTurnParams{ID: turn.ID, ExitCode: &exit, LogKey: logKey, Usage: usage}); err != nil {
			return err
		}
		var source *string
		if res.Usage.CostSource != "" {
			source = &res.Usage.CostSource
		}
		if _, err = q.AddRunUsage(ctx, store.AddRunUsageParams{ID: run.ID, ToolCalls: int32(res.Usage.ToolCalls), InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens, CostUsdMicros: int64(res.Usage.CostUSD * 1e6), CostSource: source}); err != nil {
			return err
		}
		if run.State != "running" && run.State != "paused" {
			return nil
		}
		state := "idle"
		if run.State == "running" {
			role, body := "assistant", res.Final
			if waitErr != nil || code != 0 || res.IsError || res.UsageLimited {
				state, role = "failed", "system"
				body = res.Error
				if body == "" {
					body = fmt.Sprintf("The chat turn stopped (exit %d). Send another message to retry.", code)
				}
			}
			citations := []byte("[]")
			if role == "assistant" {
				if citations, err = runs.ChatCitations(ctx, q, run.ID, turn.StartedAt); err != nil {
					return err
				}
			}
			// The opening line the chat showed while the turn worked stays in
			// the conversation, as its own reply ahead of the answer.
			if opening := strings.TrimSpace(res.Opening); role == "assistant" && opening != "" && opening != strings.TrimSpace(body) {
				if _, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: role, BodyMd: opening, Harness: &run.Harness, Model: &run.Model, RunID: &run.ID, RunTurnID: &turn.ID, ContextRefs: []byte("[]"), Citations: []byte("[]")}); err != nil {
					return err
				}
			}
			msg, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: role, BodyMd: body, Harness: &run.Harness, Model: &run.Model, RunID: &run.ID, RunTurnID: &turn.ID, ContextRefs: []byte("[]"), Citations: citations, Usage: usage})
			if err != nil {
				return err
			}
			if role == "assistant" {
				if len(codeChanges) > 2 { // more than "[]"
					if err := q.SetChatMessageCodeChanges(ctx, store.SetChatMessageCodeChangesParams{ID: msg.ID, CodeChanges: codeChanges}); err != nil {
						return err
					}
				}
				for _, im := range imgs {
					if _, err := q.CreateGeneratedChatAttachment(ctx, store.CreateGeneratedChatAttachmentParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy, MessageID: &msg.ID, Filename: im.name, ContentType: im.ct, SizeBytes: im.size, BlobKey: im.key}); err != nil {
						return err
					}
					// Every generated image is kept in the person's Library too,
					// sharing the attachment's object.
					if _, err := q.CreateLibraryItem(ctx, store.CreateLibraryItemParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy, Kind: "image", Title: runs.LibraryImageTitle(chat.Title),
						Filename: im.name, ContentType: im.ct, SizeBytes: im.size, BlobKey: im.key, ChatID: &chat.ID, MessageID: &msg.ID, PersonaID: chat.PersonaID}); err != nil {
						return err
					}
				}
				bound = len(imgs) > 0
				for _, reason := range jobs.failures {
					if _, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "system", BodyMd: "Couldn't create the image: " + reason + ".", RunID: &run.ID, RunTurnID: &turn.ID, ContextRefs: []byte("[]"), Citations: []byte("[]")}); err != nil {
						return err
					}
				}
			}
		}
		if err = idleChat(ctx, q, chat, run, state); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"state": "turn_ended", "turn": turn.Seq, "exit_code": code})
		return q.AddRunEvent(ctx, store.AddRunEventParams{RunID: run.ID, Type: "lifecycle", Payload: payload})
	})
	if err != nil {
		r.Log.Error("finish chat turn", "run", c.run.ID, "err", err)
	}
	switch {
	case len(imgs) == 0:
	case err != nil || !bound:
		r.dropImageBlobs(imgs)
	default:
		// Committed: the files must not be attached to a later message too.
		names := make([]string, len(imgs))
		for i, im := range imgs {
			names[i] = im.outbox
		}
		if err := r.Backend.RemoveOutbox(context.WithoutCancel(ctx), c.run.ID.String(), names); err != nil {
			r.Log.Warn("clear chat outbox", "run", c.run.ID, "err", err)
		}
	}
}

// failChat handles setup/invocation failures before a harness result exists.
// The segment stays usable; pending/running records cannot block its next turn.
func (r *Runner) failChat(ctx context.Context, c runCtx, outcome string) {
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		chat, err := lockRunChat(ctx, q, c.run)
		if err != nil {
			return err
		}
		run, err := q.LockRun(ctx, c.run.ID)
		if err != nil {
			return err
		}
		if err = q.FinishUnfinishedChatTurns(ctx, run.ID); err != nil {
			return err
		}
		if _, err = q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "system", BodyMd: outcome, RunID: &run.ID, ContextRefs: []byte("[]"), Citations: []byte("[]")}); err != nil {
			return err
		}
		return idleChat(ctx, q, chat, run, "failed")
	})
	if err != nil {
		r.Log.Error("fail chat turn", "run", c.run.ID, "err", err)
	}
}

// expireIdleChats completes chat segments that have waited on the person
// longer than ChatIdleTTL, so a runner doesn't keep their workspace forever.
// The chat itself stays usable: its next message starts a new segment that
// gets the conversation so far as a handoff.
func (r *Runner) expireIdleChats(ctx context.Context) {
	ttl := r.Cfg.ChatIdleTTL
	if ttl <= 0 {
		ttl = DefaultChatIdleTTL
	}
	ids, err := r.q().IdleChatRuns(ctx, time.Now().Add(-ttl))
	if err != nil {
		r.Log.Warn("idle chat runs", "err", err)
		return
	}
	for _, row := range ids {
		if err := r.expireIdleChat(ctx, store.Run{ID: row.ID, WorkspaceID: row.WorkspaceID}); err != nil {
			r.Log.Warn("end idle chat", "run", row.ID, "err", err)
		}
	}
}

func (r *Runner) expireIdleChat(ctx context.Context, run store.Run) error {
	id := run.ID
	return pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// Session lock before run lock, like every chat mutation.
		chat, err := lockRunChat(ctx, q, run)
		if errors.Is(err, pgx.ErrNoRows) {
			chat = store.ChatSession{} // no chat holds this segment any more; still end it
		} else if err != nil {
			return err
		}
		if run, err = q.LockRun(ctx, id); err != nil {
			return err
		}
		// A message may have arrived since the scan.
		if run.State != "waiting" || chat.State == "running" {
			return nil
		}
		busy, err := q.HasRunningChatTurn(ctx, id)
		if err != nil || busy {
			return err
		}
		outcome := "The chat went idle."
		if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: id, State: "completed", Outcome: &outcome}); err != nil {
			return err
		}
		if err = q.DropPendingTurns(ctx, id); err != nil {
			return err
		}
		if err = q.ExpireRunApprovals(ctx, id); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"state": "chat_segment_ended", "reason": "idle"})
		if err = q.AddRunEvent(ctx, store.AddRunEventParams{RunID: id, Type: "lifecycle", Payload: payload}); err != nil {
			return err
		}
		if chat.CurrentRunID != nil && *chat.CurrentRunID == id && slices.Contains([]string{"idle", "failed"}, chat.State) {
			if _, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, State: chat.State}); err != nil {
				return err
			}
		}
		return runs.RecomputeAgentStatus(ctx, q, run.AgentID)
	})
}
