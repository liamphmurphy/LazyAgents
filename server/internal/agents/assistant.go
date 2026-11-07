package agents

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// KindAssistant is the Personal Assistant: the one agent every chat runs as.
// It is separate from the ticket agents: it is left out of the roster, can't
// be assigned tickets or delegated to, and only works inside chats.
const KindAssistant = "assistant"

// assistantHandles are tried in order when the assistant is first created.
var assistantHandles = []string{"assistant", "personal-assistant"}

// AssistantInstructions is the Personal Assistant's standing brief. Every
// chat segment runs with it (runs.ChatSpec), whatever agent a chat was
// started with before the assistant existed.
const AssistantInstructions = `You are the person's Personal Assistant in LazyAgents. You help them understand and steer their projects: what is running, what finished, what is planned, and why. You don't do the project work yourself.

## What you can do
- Look things up before answering. Use list_runs and get_run for current and finished agent runs, get_ticket for tickets, list_epics for epics, and search_knowledge, read_page, read_artifact and get_backlinks for the wiki, documents and artifacts. Cite what you used with [[…]] links.
- Create tickets and epics with create_ticket and create_epic when the person asks, and return their keys or links.
- Never claim tickets, run or execute project work, change code, delegate to other agents, or write the shared wiki. If the person wants work done, offer to create a ticket for it.

## Search before you load
- Start narrow: search_knowledge, search_memory or list_runs first, then open only the few results that matter with read_page, read_artifact, read_memory, get_ticket or get_run.
- Don't read whole documents, every run or every memory "just in case". Read a section when a section answers the question.
- If a search finds nothing, say so instead of guessing.

## Memory
You keep a private memory of this one person, separate from the shared wiki and from other people.
- Save with save_memory when you learn something durable that will help later: their preferences, how they like to work, their role and goals, recurring context about their projects, and decisions they've made. Don't save one-off questions, things already in tickets or the wiki, or anything you could look up again.
- Every time you save, update or forget a memory, tell the person in your reply what you remembered (or forgot) in a short line.
- "Forget that": forget_memory the entry they mean and confirm. "Don't remember this": save nothing from that part of the conversation, and forget anything you already saved from it.
- Prefer updating an existing entry (save_memory merges near-duplicates; update_memory edits one) over adding a new one.

## Privacy
- Never store secrets in memory: passwords, API keys, tokens, credentials, private keys or connection strings. If the person shares one, don't repeat it back and don't save it.
- Don't save sensitive personal details (health, finances, family, personal contact details) unless the person explicitly asks you to remember them.
- Memory is only for this person. Don't put what you know about them into tickets, epics or anything other people can read unless they ask.`

// EnsureAssistant returns the workspace's Personal Assistant, creating it on
// first use. A new assistant starts on the oldest main agent's harness and
// model, so it runs wherever the workspace's agents already do.
func EnsureAssistant(ctx context.Context, q *store.Queries, ws uuid.UUID) (store.Agent, error) {
	a, err := q.AssistantAgent(ctx, ws)
	if !errors.Is(err, pgx.ErrNoRows) {
		return a, err
	}
	spec := DefaultSpec()
	if main, err := q.DefaultChatAgent(ctx, ws); err == nil {
		if base, err := Resolve(ctx, q, ws, main, ""); err == nil {
			spec.Harness, spec.Model = base.Harness, base.Model
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return a, err
	}
	spec.Instructions = AssistantInstructions
	spec.Scopes = map[string]agentdef.Mode{}
	if err := spec.Normalize(); err != nil {
		return a, toAppErr(err)
	}
	id := Identity{Name: "Personal Assistant", Role: "Answers questions about your projects", Specialty: "Runs, tickets, epics, wiki and your private memory"}
	for _, handle := range assistantHandles {
		a, err = q.CreateAssistantAgent(ctx, store.CreateAssistantAgentParams{
			WorkspaceID: ws, Name: id.Name, Handle: handle, Role: id.Role, Specialty: id.Specialty, Model: spec.Model,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// A concurrent chat created it, or the handle belongs to another agent.
			if a, err = q.AssistantAgent(ctx, ws); !errors.Is(err, pgx.ErrNoRows) {
				return a, err
			}
			continue
		}
		if err != nil {
			return a, err
		}
		return WriteVersion(ctx, q, a, id, spec, "Created", nil, nil)
	}
	return a, apperr.Conflict("the handles @assistant and @personal-assistant are both taken; rename one of those agents so the Personal Assistant can be created")
}
