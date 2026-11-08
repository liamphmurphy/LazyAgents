package runs

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// MaxChatImageJobsPerTurn caps generate_image requests in one chat turn.
const MaxChatImageJobsPerTurn = 2

// MaxChatImagePromptRunes bounds an image prompt (and chat_image_jobs.prompt).
const MaxChatImagePromptRunes = 4000

// QueueChatImage records a generate_image request from a chat turn on a
// harness that can't make images itself. The runner creates the image with a
// one-shot Codex step once the turn's reply is done. The capability is read
// fresh from the chat's persona, so turning images off applies at once.
func (s *Service) QueueChatImage(ctx context.Context, ws, runID uuid.UUID, prompt string) (store.ChatImageJob, error) {
	var job store.ChatImageJob
	prompt = strings.TrimSpace(prompt)
	if n := utf8.RuneCountInString(prompt); n < 1 || n > MaxChatImagePromptRunes {
		return job, apperr.Invalid("prompt", "provide 1–4000 characters")
	}
	q := store.New(s.Pool)
	run, err := q.GetRun(ctx, store.GetRunParams{ID: runID, WorkspaceID: ws})
	if err != nil {
		return job, err
	}
	if run.Purpose != "chat" {
		return job, apperr.Conflict("generate_image only works in a chat")
	}
	if run.State != "running" {
		return job, apperr.Conflict("generate_image only works while you are answering a message")
	}
	if run.Harness == agentdef.HarnessCodex {
		return job, apperr.Conflict("This chat runs on Codex: use your own image generation tool, then copy the image into .lazyagents/outbox/. generate_image is for other models.")
	}
	chat, err := q.ChatSessionForRun(ctx, &run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, apperr.Conflict("this chat run is no longer attached to a chat")
	}
	if err != nil {
		return job, err
	}
	// The Codex step shares the run's token and sees its tools; it must not
	// queue more images for itself.
	if running, err := q.RunningChatImageJobs(ctx, run.ID); err != nil {
		return job, err
	} else if len(running) > 0 {
		return job, apperr.Conflict("An image is being made right now; generate_image isn't available to that step")
	}
	caps, err := chatCaps(ctx, q, chat)
	if err != nil {
		return job, err
	}
	if !caps.Images {
		return job, apperr.Conflict("This persona can't generate images; tell the person it isn't enabled.")
	}
	catalog, err := s.ChatCatalog().List(ctx, q, ws)
	if err != nil {
		return job, err
	}
	model, reason := "", "Codex isn't available"
	for _, h := range catalog {
		if h.Harness != agentdef.HarnessCodex {
			continue
		}
		if !h.Available {
			reason = h.UnavailableReason
			break
		}
		for _, m := range h.Models {
			if model == "" || m.Default {
				model = m.ID
			}
		}
		if model == "" {
			reason = "no Codex models are available"
		}
	}
	if model == "" {
		return job, apperr.Conflict("Image generation needs Codex: %s", reason)
	}
	turn, err := q.RunningChatTurn(ctx, run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, apperr.Conflict("generate_image only works while you are answering a message")
	}
	if err != nil {
		return job, err
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tq := store.New(tx)
		if _, err := tq.LockChatSession(ctx, store.LockChatSessionParams{ID: chat.ID, WorkspaceID: ws}); err != nil {
			return err
		}
		n, err := tq.CountChatImageJobsForTurn(ctx, store.CountChatImageJobsForTurnParams{RunID: run.ID, TurnID: turn.ID})
		if err != nil {
			return err
		}
		if n >= MaxChatImageJobsPerTurn {
			return apperr.Conflict("You can request at most %d images in one reply; tell the person to ask again for more.", MaxChatImageJobsPerTurn)
		}
		job, err = tq.CreateChatImageJob(ctx, store.CreateChatImageJobParams{WorkspaceID: ws, RunID: run.ID, TurnID: turn.ID, ChatID: chat.ID, Prompt: prompt, Model: model})
		return err
	})
	return job, err
}
