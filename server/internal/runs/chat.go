package runs

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/auth"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/links"
	"github.com/liamphmurphy/vesper/server/internal/personas"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// ChatCaps are what a chat's persona lets it do beyond answering, loaded fresh
// from the chat and its persona whenever a segment is created or checked. Only
// these structured values widen the frozen spec (ADR "Assistant personas");
// a persona's free text never does.
type ChatCaps struct {
	// Code is the persona's can_code: the chat may write and run code in its
	// workspace. RepoURL (fixed when the chat is created) is the repository
	// checked out there; without one the workspace starts empty, a throwaway
	// scratch space with no git credentials.
	Code    bool
	RepoURL string
	// WorkBranches are the branches the chat's persona has pushed so far, in
	// the order it made them; it makes them as changes are asked for.
	WorkBranches []string
	Images       bool
	// Connectors are the slugs of the MCP connectors the persona may use here.
	Connectors []string
	// Secrets is whether the persona may use any of the person's API secrets
	// here; their values stay in Vesper (agentapi.SecretsHandler).
	Secrets bool
}

// Coding reports whether the chat may write and run code in its workspace.
func (c ChatCaps) Coding() bool { return c.Code }

// RepoCoding reports whether the chat works in a checked-out repository,
// with branches and pushes, rather than an empty scratch workspace.
func (c ChatCaps) RepoCoding() bool { return c.Code && c.RepoURL != "" }

// codeScopes are what a coding chat is granted; every other scope stays denied.
var codeScopes = []string{"repo:read", "repo:write", "tests:run"}

// ChatSpec freezes a conversation's capabilities after project overrides.
func ChatSpec(r agentdef.Resolved, caps ChatCaps) agentdef.Resolved {
	r.Scopes = append([]agentdef.ResolvedScope{}, r.Scopes...)
	for i := range r.Scopes {
		r.Scopes[i].Mode = agentdef.Denied
	}
	coding := caps.Coding()
	if coding {
		// The strictest matching entry wins, so a denied wildcard like repo:*
		// would cancel the grants below: drop every entry that covers one.
		r.Scopes = slices.DeleteFunc(r.Scopes, func(s agentdef.ResolvedScope) bool {
			return slices.ContainsFunc(codeScopes, func(c string) bool {
				_, covers := agentdef.ModeFor(map[string]agentdef.Mode{s.Scope: agentdef.Denied}, c)
				return covers
			})
		})
		for _, scope := range codeScopes {
			r.Scopes = append(r.Scopes, agentdef.ResolvedScope{Scope: scope, Mode: agentdef.Allowed, Source: agentdef.FromPersona})
		}
	}
	r.Setup = []string{}
	r.MCPServers = []agentdef.MCPServer{}
	for _, slug := range caps.Connectors {
		r.MCPServers = append(r.MCPServers, agentdef.MCPServer{Name: slug, URL: "vesper", Connector: true})
	}
	if caps.Secrets {
		r.MCPServers = append(r.MCPServers, agentdef.MCPServer{Name: agentdef.SecretsMCPName, URL: "vesper", Connector: true})
	}
	// web_search turns on the harness's own web search and page fetch tools
	// (Claude Code WebSearch/WebFetch, Codex cached web search) for every chat,
	// coding or not; WebFetch stays on the CheckTool allowlist. See
	// agentdef.OptionWebSearch and ADR "Assistant web access".
	r.HarnessOptions = map[string]string{"sandbox": "read-only", "readonly": "true", agentdef.OptionWebSearch: "true"}
	if coding {
		r.HarnessOptions = map[string]string{"sandbox": "workspace-write", agentdef.OptionWebSearch: "true"}
	}
	// Codex generates images with its own tool and copies them into the
	// workspace outbox (the Codex harness grants write to only that directory
	// for an image-only chat). Other harnesses can't, and never receive the option.
	images := caps.Images && r.Harness == agentdef.HarnessCodex
	if images {
		r.HarnessOptions[agentdef.OptionImageGeneration] = "true"
	}
	r.Repository = nil
	r.Fallbacks = nil // A chat segment never switches harness or model implicitly.
	// Every chat is the Personal Assistant, whichever agent it was started with.
	r.Instructions = agents.AssistantInstructionsFor(coding, caps.RepoCoding(), caps.WorkBranches, caps.Images, r.Harness == agentdef.HarnessCodex)
	return r
}

// chatCaps loads a chat's capabilities from the persona answering it (its
// speaker, else its host), which must belong to the chat's creator (a deleted
// persona gives none).
func chatCaps(ctx context.Context, q *store.Queries, chat store.ChatSession) (ChatCaps, error) {
	id := ChatSpeakerID(chat)
	if id == nil {
		return ChatCaps{}, nil
	}
	p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *id, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatCaps{}, nil
	}
	if err != nil {
		return ChatCaps{}, err
	}
	// A repository the persona doesn't list (any more) revokes coding for this
	// chat, rather than leaving a shell beside a checkout it may no longer
	// push to. A chat without a repository codes in a scratch workspace.
	code := p.CanCode && (chat.RepoUrl == nil || slices.Contains(p.Repos, *chat.RepoUrl))
	rows, err := q.ConnectorsForPersona(ctx, store.ConnectorsForPersonaParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy, PersonaID: p.ID, AllowedIds: chat.ConnectorIds})
	if err != nil {
		return ChatCaps{}, err
	}
	slugs := make([]string, len(rows))
	for i, c := range rows {
		slugs[i] = c.Slug
	}
	secrets, err := q.UserSecretsForPersona(ctx, store.UserSecretsForPersonaParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy, PersonaID: p.ID, AllowedIds: chat.SecretIds})
	if err != nil {
		return ChatCaps{}, err
	}
	return ChatCaps{Code: code, RepoURL: deref(chat.RepoUrl), WorkBranches: chat.WorkBranches, Images: p.CanGenerateImages, Connectors: slugs, Secrets: len(secrets) > 0}, nil
}

// sameCapabilities reports whether two chat specs grant the same persona
// capabilities: the modes of the scopes a coding chat is granted, and the
// sandbox, read-only, image generation and web search options. Instructions
// and the rest of the assistant's definition are left out on purpose, so
// editing them never ends a segment, and neither does the persona pushing a
// new branch.
func sameCapabilities(a, b agentdef.Resolved) bool {
	key := func(r agentdef.Resolved) string {
		modes := map[string]agentdef.Mode{}
		for _, scope := range codeScopes {
			modes[scope] = r.Mode(scope)
		}
		opts := map[string]string{}
		for _, k := range []string{"sandbox", "readonly", agentdef.OptionImageGeneration, agentdef.OptionWebSearch} {
			opts[k] = r.HarnessOptions[k]
		}
		connectors := []string{}
		for _, m := range r.MCPServers {
			if m.Connector {
				connectors = append(connectors, m.Name)
			}
		}
		slices.Sort(connectors)
		raw, _ := json.Marshal(map[string]any{"modes": modes, "options": opts, "connectors": connectors})
		return string(raw)
	}
	return key(a) == key(b)
}

// ChatInput opens an idle conversation with the Personal Assistant; its first
// message creates the segment run. An empty Harness/Model keeps what the
// assistant and project resolve to.
type ChatInput struct {
	Project string
	Title   string
	Harness string
	Model   string
	// Effort is a reasoning effort the model offers; empty is automatic.
	Effort string
	// PersonaID is one of the creator's personas; nil means their default
	// persona (none when they have no default). Fixed for the chat's life.
	PersonaID *uuid.UUID
	// RepoURL is one of the persona's repos; empty takes the first when the
	// persona can code. Fixed for the chat's life.
	RepoURL string
	// Hidden keeps the chat out of chat lists, search and the dock, for a chat
	// that lives behind another screen. Fixed for the chat's life.
	Hidden bool
	// Temporary runs the chat without the person's memory or past chats, keeps
	// it out of search, and the runner deletes it once it has sat idle for a
	// day (runner.TemporaryChatTTL). Fixed for the chat's life.
	Temporary bool
	// NoRepo keeps a coding persona's chat out of its repositories.
	NoRepo bool
	// CommitEmail is one of the creator's addresses for the chat's commits;
	// empty leaves it unpicked (their primary is used). Only with a repository.
	CommitEmail string
	// ConnectorIDs, when set, narrows the persona's connectors for this chat.
	ConnectorIDs []uuid.UUID
	// SpaceID puts the chat in one of the creator's spaces, whose defaults
	// fill in the persona, project and connectors not given here.
	SpaceID *uuid.UUID
	Message *ChatMessageInput
}

// ChatMessageInput is a single human prompt. A non-empty Harness/Model is
// validated against the catalog and stored on the chat; the runner applies it
// in LAZYAGENTS-10, so an existing segment keeps running its own harness/model.
// ContextRefs are resolved inside the workspace and delivered to the turn.
type ChatMessageInput struct {
	Body    string
	Harness string
	Model   string
	// Effort switches the reasoning effort from this message on; empty keeps
	// it, chatmodels.EffortDefault clears it.
	Effort      string
	ContextRefs []ChatContextRef
	// AttachmentIDs are the poster's own unattached uploads (chat_attachments);
	// they're bound to the new message and delivered to its turn.
	AttachmentIDs []uuid.UUID
	// Research asks for a plan to approve instead of an answer.
	Research bool
}

// ChatCatalog is the harness/model catalog chats validate against.
func (s *Service) ChatCatalog() chatmodels.Catalog {
	return chatmodels.Catalog{RequireKey: s.RequireHarnessKey, Secrets: s.Secrets, Discover: s.ModelDiscovery, PlanModels: s.PlanModels}
}

// StartChat creates a chat and optionally queues its first message atomically.
func (s *Service) StartChat(ctx context.Context, ws, user uuid.UUID, in ChatInput) (store.ChatSession, error) {
	var out store.ChatSession
	in.Title = strings.TrimSpace(in.Title)
	if len([]rune(in.Title)) > 200 {
		return out, apperr.Invalid("title", "must be at most 200 characters")
	}
	err := s.tx(ctx, func(q *store.Queries) error {
		var err error
		out, err = s.startChat(ctx, q, ws, user, in)
		return err
	})
	return out, err
}

func (s *Service) startChat(ctx context.Context, q *store.Queries, ws, user uuid.UUID, in ChatInput) (store.ChatSession, error) {
	var out store.ChatSession
	err := func() error {
		space, err := chatSpace(ctx, q, ws, user, &in)
		if err != nil {
			return err
		}
		// A chat needs no project; one named here is attached from the start.
		var p store.Project
		var projectID *uuid.UUID
		if in.Project != "" {
			if p, err = lockAttachableProject(ctx, q, ws, in.Project); err != nil {
				return err
			}
			projectID = &p.ID
		}
		a, err := agents.EnsureAssistant(ctx, q, ws)
		if err != nil {
			return err
		}
		spec, err := agents.Resolve(ctx, q, ws, a, p.Key)
		if err != nil {
			return err
		}
		sel, err := s.ChatCatalog().Resolve(ctx, q, ws, chatmodels.Selection{Harness: spec.Harness, Model: spec.Model},
			chatmodels.Selection{Harness: in.Harness, Model: in.Model, Effort: in.Effort})
		if err != nil {
			return err
		}
		var persona *store.Persona
		if in.PersonaID != nil {
			_, err := q.KeySharePersona(ctx, store.KeySharePersonaParams{ID: *in.PersonaID, WorkspaceID: ws, UserID: user})
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Invalid("persona_id", "unknown persona")
			}
			if err != nil {
				return err
			}
			pp, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *in.PersonaID, WorkspaceID: ws, UserID: user})
			if err != nil {
				return err
			}
			persona = &pp
		} else if pp, err := q.DefaultPersona(ctx, store.DefaultPersonaParams{WorkspaceID: ws, UserID: user}); err == nil {
			// The same default CreateChatSession would pick; resolved here to read its repos.
			persona = &pp
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Passed explicitly so the default can't change between the lookup above
		// and the insert.
		var personaID *uuid.UUID
		if persona != nil {
			personaID = &persona.ID
		}
		repo, err := chatRepo(persona, in.RepoURL)
		if err != nil {
			return err
		}
		if in.NoRepo {
			repo = ""
		}
		title := in.Title
		if title == "" && in.Message != nil {
			title = firstLine(in.Message.Body, 80)
		}
		out, err = q.CreateChatSession(ctx, store.CreateChatSessionParams{WorkspaceID: ws, ProjectID: projectID, Title: title, CreatedBy: user, AgentID: a.ID, Harness: sel.Harness, Model: sel.Model, Effort: sel.Effort, PersonaID: personaID, Hidden: in.Hidden, SpaceID: space, Temporary: in.Temporary})
		if err != nil {
			return err
		}
		if in.ConnectorIDs != nil {
			if out, err = q.SetChatConnectors(ctx, store.SetChatConnectorsParams{ID: out.ID, ConnectorIds: in.ConnectorIDs}); err != nil {
				return err
			}
		}
		if repo != "" {
			// No branch yet: the persona makes one when a change is asked for.
			out, err = q.SetChatRepo(ctx, store.SetChatRepoParams{ID: out.ID, RepoUrl: &repo})
			if err != nil {
				return err
			}
		}
		if in.CommitEmail != "" {
			if out, err = setChatCommitEmail(ctx, q, out, in.CommitEmail); err != nil {
				return err
			}
		}
		if in.Message != nil {
			out, _, err = postChatMessage(ctx, q, s.ChatCatalog(), out, *in.Message, s.HandoffChars)
		}
		return err
	}()
	return out, err
}

// chatRepo picks the repository a new chat works in: the requested one, which
// must be among a coding persona's repos (compared after normalization), or
// the persona's first. "" means the chat has no repository.
func chatRepo(persona *store.Persona, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		if persona != nil && persona.CanCode && len(persona.Repos) > 0 {
			return persona.Repos[0], nil
		}
		return "", nil
	}
	if persona == nil || !persona.CanCode {
		return "", apperr.Invalid("repo_url", "only a persona that can code can work in a repository")
	}
	url, err := personas.NormalizeRepoURL(requested)
	if err != nil || !slices.Contains(persona.Repos, url) {
		return "", apperr.Invalid("repo_url", "choose one of the persona's repositories")
	}
	return url, nil
}

// PostChatMessage serializes posts on the session row, including first-run creation.
// ConflictError is mapped to HTTP 409 by the shared API error handler.
func (s *Service) PostChatMessage(ctx context.Context, ws, user, id uuid.UUID, in ChatMessageInput) (store.ChatMessage, error) {
	var out store.ChatMessage
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		_, out, err = postChatMessage(ctx, q, s.ChatCatalog(), chat, in, s.HandoffChars)
		return err
	})
	if err == nil && s.Transcripts != nil {
		s.Transcripts.Prefetch(in.Body)
	}
	return out, err
}

// assistantSpec is the Personal Assistant and its definition resolved for the
// project, before ChatSpec freezes it for one segment.
func assistantSpec(ctx context.Context, q *store.Queries, ws uuid.UUID, project string) (store.Agent, agentdef.Resolved, error) {
	a, err := agents.EnsureAssistant(ctx, q, ws)
	if err != nil {
		return a, agentdef.Resolved{}, err
	}
	spec, err := agents.Resolve(ctx, q, ws, a, project)
	return a, spec, err
}

func lockChat(ctx context.Context, q *store.Queries, ws, user, id uuid.UUID) (store.ChatSession, error) {
	chat, err := q.LockChatSession(ctx, store.LockChatSessionParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && chat.CreatedBy != user) {
		return chat, apperr.ErrNotFound
	}
	return chat, err
}

// firstTurnNote is appended to a conversation's first turn prompt only; the
// person never sees it in the stored message.
const firstTurnNote = "\n\n(Vesper: this is the first message of a new conversation. Call set_chat_title once with a concise title of 2 to 6 words that says what the person wants, then answer as usual. Don't mention the title in your reply.)"

func postChatMessage(ctx context.Context, q *store.Queries, cat chatmodels.Catalog, chat store.ChatSession, in ChatMessageInput, handoffChars int) (store.ChatSession, store.ChatMessage, error) {
	var msg store.ChatMessage
	body := strings.TrimSpace(in.Body)
	if body == "" || len([]rune(body)) > 100000 {
		return chat, msg, apperr.Invalid("body", "provide 1–100000 characters")
	}
	if chat.Archived {
		return chat, msg, apperr.Conflict("the chat is archived")
	}
	// A message during a running reply interrupts it (see chat_interrupt.go);
	// one interruption at a time, while the stopped process exits.
	interrupt := chat.State == "running" && chat.CurrentRunID != nil
	if interrupt && chat.InterruptMessageID != nil {
		return chat, msg, apperr.Conflict("still stopping the reply for your last message; send this again in a moment")
	}
	p, err := chatProject(ctx, q, chat)
	if err != nil {
		return chat, msg, err
	}
	if p.Archived {
		return chat, msg, apperr.Conflict("restore the archived project before chatting")
	}
	// Validate the selection and the refs before anything is written; a 422
	// leaves the stored selection unchanged (the whole transaction rolls back).
	cur := chatmodels.Selection{Harness: chat.Harness, Model: chat.Model, Effort: chat.Effort}
	sel, err := cat.Resolve(ctx, q, chat.WorkspaceID, cur, chatmodels.Selection{Harness: in.Harness, Model: in.Model, Effort: in.Effort})
	if err != nil {
		return chat, msg, err
	}
	if len(in.AttachmentIDs) > MaxChatAttachments {
		return chat, msg, apperr.Invalid("attachment_ids", "attach at most %d files", MaxChatAttachments)
	}
	var attachIDs []uuid.UUID
	for _, id := range in.AttachmentIDs {
		if !slices.Contains(attachIDs, id) {
			attachIDs = append(attachIDs, id)
		}
	}
	docs, err := ResolveChatRefs(ctx, q, chat.WorkspaceID, chat.ID, in.ContextRefs)
	if err != nil {
		return chat, msg, err
	}
	refs := make([]ChatContextRef, len(docs))
	for i, d := range docs {
		refs[i] = d.Ref
	}
	rawRefs, err := json.Marshal(refs)
	if err != nil {
		return chat, msg, err
	}
	if sel != cur {
		if chat, err = q.SetChatSelection(ctx, store.SetChatSelectionParams{ID: chat.ID, Harness: sel.Harness, Model: sel.Model, Effort: sel.Effort}); err != nil {
			return chat, msg, err
		}
	}
	if chat.Title == "" {
		// An empty chat has no title yet; the first message names it until the agent does.
		total, err := q.CountChatHistory(ctx, chat.ID)
		if err != nil {
			return chat, msg, err
		}
		if total == 0 {
			if chat, err = q.SetChatTitle(ctx, store.SetChatTitleParams{ID: chat.ID, Title: firstLine(body, 80)}); err != nil {
				return chat, msg, err
			}
		}
	}
	prompt := body
	if interrupt {
		var stopped bool
		var dropped *store.RunTurn
		if chat, stopped, dropped, err = stopChatTurn(ctx, q, chat); err != nil {
			return chat, msg, err
		}
		if !stopped {
			// The reply's process is still exiting: the message waits on the
			// chat, and the runner starts its turn (ResumeInterruptedChat).
			appended, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "user", BodyMd: body, ContextRefs: rawRefs, Citations: []byte("[]")})
			if err != nil {
				return chat, msg, err
			}
			msg = store.ChatMessage(appended)
			if chat, err = q.SetChatInterrupt(ctx, store.SetChatInterruptParams{ID: chat.ID, InterruptMessageID: &msg.ID}); err != nil {
				return chat, msg, err
			}
			return chat, msg, bindChatAttachments(ctx, q, chat, msg, attachIDs)
		}
		// Nothing had started: this turn replaces the stopped one.
		note := replacedTurnNote(dropped)
		if note == "" {
			if note, err = interruptNote(ctx, q, chat, lastUserSeq, nil, nil); err != nil {
				return chat, msg, err
			}
		}
		prompt = note + body
	} else if chat.InterruptMessageID != nil {
		// Left by a stop or failure that never resumed it.
		if chat, err = q.SetChatInterrupt(ctx, store.SetChatInterruptParams{ID: chat.ID}); err != nil {
			return chat, msg, err
		}
	}
	if in.Research {
		prompt = researchPlanNote + prompt
	}
	// A mention picks who answers (and queues the rest); otherwise whoever
	// spoke last carries on. The message may also set how long personas keep
	// handing the conversation to each other.
	speaker, queue, err := routeHumanMessage(ctx, q, chat, body)
	if err != nil {
		return chat, msg, err
	}
	if chat, err = q.SetChatSpeaker(ctx, store.SetChatSpeakerParams{ID: chat.ID, SpeakerPersonaID: speaker, QueuedPersonaIds: queue, RelayTurns: 0, RelayLimit: int32(RelayLimitIn(body))}); err != nil {
		return chat, msg, err
	}
	var run store.Run
	var turn store.RunTurn
	if in.Research {
		chat, run, turn, err = startChatTurn(ctx, q, chat, p.Key, prompt, "xhigh", false, handoffChars)
	} else {
		chat, run, turn, err = startClassifiedChatTurn(ctx, q, chat, p.Key, prompt, body, interrupt, false, handoffChars)
	}
	if err != nil {
		return chat, msg, err
	}
	appended, err := q.AppendChatMessage(ctx, store.AppendChatMessageParams{SessionID: chat.ID, Role: "user", BodyMd: body, Harness: &run.Harness, Model: &run.Model, RunID: &run.ID, RunTurnID: &turn.ID, ContextRefs: rawRefs, Citations: []byte("[]")})
	if err != nil {
		return chat, msg, err
	}
	msg = store.ChatMessage(appended)
	return chat, msg, bindChatAttachments(ctx, q, chat, msg, attachIDs)
}

// bindChatAttachments claims the uploads a person's message carries.
func bindChatAttachments(ctx context.Context, q *store.Queries, chat store.ChatSession, msg store.ChatMessage, attachIDs []uuid.UUID) error {
	if len(attachIDs) == 0 {
		return nil
	}
	// Only the chat owner's own uploads that no message has claimed yet; any
	// other id fails the whole post (the transaction rolls back).
	bound, err := q.AttachChatAttachments(ctx, store.AttachChatAttachmentsParams{MessageID: &msg.ID, Ids: attachIDs, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if err != nil {
		return err
	}
	if len(bound) != len(attachIDs) {
		return apperr.Invalid("attachment_ids", "upload the files again: an attachment is unknown or already sent")
	}
	return nil
}

// startChatTurn queues a turn for the chat's current speaker: on the current
// segment when it is idle and its capabilities still match the speaker's, or
// on a fresh segment that gets the conversation so far as a handoff. body is
// the new message (the person's, or a relay's hand-over note). relay marks a turn one persona handed to
// another, which has no new message from the person. level is the effort an
// automatic chat aims for this turn. The caller holds the chat lock.
func startChatTurn(ctx context.Context, q *store.Queries, chat store.ChatSession, project, body, level string, relay bool, handoffChars int) (store.ChatSession, store.Run, store.RunTurn, error) {
	var run store.Run
	var turn store.RunTurn
	if id := ChatSpeakerID(chat); id != nil {
		// Recency orders the persona list and picks the successor when the default is deleted.
		if err := q.TouchPersonaUsed(ctx, *id); err != nil {
			return chat, run, turn, err
		}
	}
	prompt := body
	fresh := chat.CurrentRunID == nil
	effort := chat.Effort
	if effort == "" {
		effort = chatmodels.AutoEffort(chat.WorkspaceID, chat.Harness, chat.Model, level)
	}
	if !fresh {
		var err error
		run, err = q.LockRun(ctx, *chat.CurrentRunID)
		if err != nil {
			return chat, run, turn, err
		}
		// Only an idle/waiting segment may switch; a busy one conflicts here.
		if run.State != "waiting" {
			return chat, run, turn, apperr.Conflict("the chat run is not ready for a message")
		}
		caps, err := chatCaps(ctx, q, chat)
		if err != nil {
			return chat, run, turn, err
		}
		// The segment's frozen spec was built from the speaker's capabilities
		// then; if they have changed since (an edit, or another persona with
		// other capabilities answering), a fresh segment gets the current ones.
		// Turning one off takes effect on the next message.
		changed := false
		if chat.Harness == run.Harness {
			_, base, err := assistantSpec(ctx, q, chat.WorkspaceID, project)
			if err != nil {
				return chat, run, turn, err
			}
			base.Harness, base.Model = run.Harness, run.Model
			have, err := Spec(run)
			if err != nil {
				return chat, run, turn, err
			}
			changed = !sameCapabilities(ChatSpec(base, caps), have)
		}
		switch {
		case changed:
			outcome := "Chat capabilities changed."
			if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
				return chat, run, turn, err
			}
			if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "chat_segment_ended", "reason": "capabilities_changed", "harness": chat.Harness}); err != nil {
				return chat, run, turn, err
			}
			fresh = true
		case chat.Harness != run.Harness:
			// Every harness switch starts a fresh session, even back to an earlier
			// harness: the native session is stale. The runner sweep removes the
			// ended segment's workspace.
			outcome := "Chat switched to " + chat.Harness + "."
			if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
				return chat, run, turn, err
			}
			if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "chat_segment_ended", "reason": "harness_switch", "harness": chat.Harness}); err != nil {
				return chat, run, turn, err
			}
			fresh = true
		case chat.Model != run.Model:
			// Same harness: keep the CLI session and workspace, change the model.
			if err = q.SetRunModel(ctx, store.SetRunModelParams{ID: run.ID, Model: chat.Model}); err != nil {
				return chat, run, turn, err
			}
			run.Model = chat.Model
		}
		if !fresh {
			// Same harness: the next turn's invocation picks up the new effort.
			have, err := Spec(run)
			if err != nil {
				return chat, run, turn, err
			}
			if have.HarnessOptions["effort"] != effort {
				if err = q.SetRunEffort(ctx, store.SetRunEffortParams{ID: run.ID, Effort: effort}); err != nil {
					return chat, run, turn, err
				}
			}
		}
	}
	if fresh {
		// Chats started before the Personal Assistant existed move to it at their next segment.
		a, spec, err := assistantSpec(ctx, q, chat.WorkspaceID, project)
		if err != nil {
			return chat, run, turn, err
		}
		if chat.AgentID != a.ID {
			if err = q.SetChatAgent(ctx, store.SetChatAgentParams{ID: chat.ID, AgentID: a.ID}); err != nil {
				return chat, run, turn, err
			}
			chat.AgentID = a.ID
		}
		caps, err := chatCaps(ctx, q, chat)
		if err != nil {
			return chat, run, turn, err
		}
		spec.Harness, spec.Model = chat.Harness, chat.Model
		spec = ChatSpec(spec, caps)
		if effort != "" {
			spec.HarnessOptions["effort"] = effort
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			return chat, run, turn, err
		}
		run, err = q.CreateRun(ctx, store.CreateRunParams{WorkspaceID: chat.WorkspaceID, AgentID: a.ID, Mode: "managed", State: "queued", ResolvedSpec: raw, DefinitionVersionID: a.CurrentDefinitionVersionID, Harness: spec.Harness, Model: spec.Model, Purpose: "chat"})
		if err != nil {
			return chat, run, turn, err
		}
		if err = q.SetRunDispatcher(ctx, store.SetRunDispatcherParams{ID: run.ID, DispatchedByUserID: &chat.CreatedBy}); err != nil {
			return chat, run, turn, err
		}
		// A segment that follows earlier messages gets them as its first prompt.
		total, err := q.CountChatHistory(ctx, chat.ID)
		if err != nil {
			return chat, run, turn, err
		}
		if total == 0 && !relay {
			prompt += firstTurnNote
		}
		if total > 0 {
			rows, err := q.ListRecentChatMessages(ctx, store.ListRecentChatMessagesParams{SessionID: chat.ID, RowLimit: handoffRows, MaxChars: handoffRowChars})
			if err != nil {
				return chat, run, turn, err
			}
			hist := make([]HandoffMessage, len(rows))
			for i, r := range rows {
				hist[i] = HandoffMessage{Seq: r.Seq, Role: r.Role, Harness: deref(r.Harness), Model: deref(r.Model), Persona: r.PersonaName, Body: r.BodyMd}
			}
			footer := handoffFooter
			if relay {
				footer = relayFooter
			}
			prompt = renderHandoff(hist, int(total), handoffChars, footer, prompt)
		}
	}
	state := "running"
	if run.WorkspaceVolume == nil {
		state = "queued"
	}
	if _, err := q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: state}); err != nil {
		return chat, run, turn, err
	}
	turn, err := q.StartTurn(ctx, store.StartTurnParams{RunID: run.ID, Trigger: "comment", InputMessage: prompt})
	if err != nil {
		return chat, run, turn, err
	}
	chat, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, CurrentRunID: &run.ID, State: "running"})
	if err != nil {
		return chat, run, turn, err
	}
	return chat, run, turn, event(ctx, q, run.ID, "lifecycle", map[string]any{"state": state, "purpose": "chat", "chat_id": chat.ID})
}

// CancelChat stops only the current turn. While its process exits, new posts
// conflict; the runner then returns the same segment and workspace to idle.
func (s *Service) CancelChat(ctx context.Context, ws, user, id uuid.UUID) (store.ChatSession, error) {
	var out store.ChatSession
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		out = chat
		if chat.State != "running" || chat.CurrentRunID == nil {
			return apperr.Conflict("no turn is running")
		}
		if chat.InterruptMessageID != nil {
			// Stop also drops an interruption still waiting for its turn; the
			// message stays in the conversation, unanswered.
			if chat, err = q.SetChatInterrupt(ctx, store.SetChatInterruptParams{ID: chat.ID}); err != nil {
				return err
			}
		}
		out, _, _, err = stopChatTurn(ctx, q, chat)
		return err
	})
	return out, err
}

// ChatCitations is the JSON for a chat message's citations: what the run read
// since the turn began (the ChatCitation schema's target, plus the exact version
// and section). Reads are deduplicated by artifact, version and section.
func ChatCitations(ctx context.Context, q *store.Queries, runID uuid.UUID, since time.Time) ([]byte, error) {
	rows, err := q.ChatTurnCitations(ctx, store.ChatTurnCitationsParams{RunID: &runID, Since: since})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		anchor := ""
		if r.Section != nil {
			anchor = *r.Section
		}
		a := store.Artifact{ID: r.ArtifactID, Type: r.Type, Title: r.Title, Path: r.Path, Number: r.Number}
		out = append(out, map[string]any{
			"target": links.ArtifactTarget(a, anchor), "artifact_id": r.ArtifactID, "title": r.Title, "path": r.Path,
			"version_id": r.VersionID, "version": r.Version, "section": r.Section,
		})
	}
	return json.Marshal(out)
}

func firstLine(body string, max int) string {
	line, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	r := []rune(strings.TrimSpace(line))
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return string(r)
}

// SetAgentChatTitle names a chat on the agent's behalf, but only while its
// title is still the automatic one (empty, or the first message's first line):
// a title the person typed is never overwritten. applied is false when blocked.
func (s *Service) SetAgentChatTitle(ctx context.Context, chatID uuid.UUID, title string) (applied bool, chat store.ChatSession, err error) {
	err = s.tx(ctx, func(q *store.Queries) error {
		chat, err = q.LockChatSessionByID(ctx, chatID)
		if err != nil {
			return err
		}
		if chat.Archived {
			return apperr.Conflict("the chat is archived")
		}
		if chat.Title != "" {
			first, err := q.FirstChatUserMessageBody(ctx, chat.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if errors.Is(err, pgx.ErrNoRows) || chat.Title != firstLine(first, 80) {
				return nil
			}
		}
		chat, err = q.SetChatTitle(ctx, store.SetChatTitleParams{ID: chat.ID, Title: title})
		applied = err == nil
		return err
	})
	return
}

// ChatPatchInput is a revision-checked rename and/or archive change.
type ChatPatchInput struct {
	Revision int32
	Title    *string
	Archived *bool
	// Project attaches this project's key to the chat; "" detaches. nil leaves it.
	Project *string
}

// UpdateChat renames, archives or restores a chat, or changes its project. Archiving ends the chat's
// current run (the runner sweeps its workspace); a restored chat starts a new
// segment on its next message. 409 on a stale revision or while a turn runs.
func (s *Service) UpdateChat(ctx context.Context, ws, user, id uuid.UUID, in ChatPatchInput) (store.ChatSession, error) {
	var out store.ChatSession
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		title, archived := chat.Title, chat.Archived
		if in.Title != nil {
			title = strings.TrimSpace(*in.Title)
			if title == "" || len([]rune(title)) > 200 {
				return apperr.Invalid("title", "must be 1–200 characters")
			}
		}
		if in.Archived != nil {
			archived = *in.Archived
		}
		if in.Title == nil && in.Archived == nil && in.Project == nil {
			return apperr.Invalid("title", "provide a title, archived or project")
		}
		if chat.Revision != in.Revision {
			return apperr.Conflict("the chat changed; reload and try again")
		}
		if chat.State == "running" {
			return apperr.Conflict("a turn is still running")
		}
		if in.Project != nil {
			var projectID *uuid.UUID
			if *in.Project != "" {
				p, err := lockAttachableProject(ctx, q, ws, *in.Project)
				if err != nil {
					return err
				}
				projectID = &p.ID
			}
			if !sameProject(chat.ProjectID, projectID) {
				// The segment's instructions name the project, so the next
				// message starts a fresh one with the conversation handed off.
				if chat, err = endChatSegment(ctx, q, chat, "The chat's project changed."); err != nil {
					return err
				}
				if chat, err = q.SetChatProject(ctx, store.SetChatProjectParams{ID: chat.ID, WorkspaceID: ws, Revision: chat.Revision, ProjectID: projectID}); err != nil {
					return err
				}
			}
		}
		if archived && !chat.Archived && chat.CurrentRunID != nil {
			run, err := q.LockRun(ctx, *chat.CurrentRunID)
			if err != nil {
				return err
			}
			if !slices.Contains(Terminal, run.State) {
				outcome := "The chat was archived."
				if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
					return err
				}
				if err = q.ExpireRunApprovals(ctx, run.ID); err != nil {
					return err
				}
				if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "completed", "outcome": outcome}); err != nil {
					return err
				}
				if err = RecomputeAgentStatus(ctx, q, run.AgentID); err != nil {
					return err
				}
			}
			if _, err = q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, State: chat.State}); err != nil {
				return err
			}
		}
		out, err = q.UpdateChatTitleArchived(ctx, store.UpdateChatTitleArchivedParams{ID: chat.ID, Title: title, Archived: archived})
		return err
	})
	return out, err
}

// SetChatCommitEmail picks which of the creator's addresses a coding chat's
// commits are authored as. The runner sets git's identity on every turn, so it
// applies from the next one; unlike UpdateChat it is allowed while a turn runs
// and needs no revision.
func (s *Service) SetChatCommitEmail(ctx context.Context, ws, user, id uuid.UUID, email string) (store.ChatSession, error) {
	var out store.ChatSession
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		out, err = setChatCommitEmail(ctx, q, chat, email)
		return err
	})
	return out, err
}

func setChatCommitEmail(ctx context.Context, q *store.Queries, chat store.ChatSession, email string) (store.ChatSession, error) {
	if chat.RepoUrl == nil {
		return chat, apperr.Invalid("commit_email", "only a chat with a repository makes commits")
	}
	u, err := q.GetUser(ctx, store.GetUserParams{ID: chat.CreatedBy, WorkspaceID: chat.WorkspaceID})
	if err != nil {
		return chat, err
	}
	own, ok := auth.OwnEmail(u, email)
	if !ok {
		return chat, apperr.Invalid("commit_email", "add %s to your profile first", strings.TrimSpace(email))
	}
	return q.SetChatCommitEmail(ctx, store.SetChatCommitEmailParams{ID: chat.ID, WorkspaceID: chat.WorkspaceID, CommitEmail: &own})
}

// lockAttachableProject is the active project a chat may attach.
func lockAttachableProject(ctx context.Context, q *store.Queries, ws uuid.UUID, key string) (store.Project, error) {
	p, err := q.LockProjectByKey(ctx, store.LockProjectByKeyParams{WorkspaceID: ws, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperr.Invalid("project", "unknown project")
	}
	if err != nil {
		return p, err
	}
	if p.Archived {
		return p, apperr.Conflict("restore the archived project before attaching it to a chat")
	}
	return p, nil
}

func sameProject(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ProjectIDOrNil is the id to scope lookups by: uuid.Nil matches no project, so
// a chat without one sees only workspace-level content.
func ProjectIDOrNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

// OptionalProject is the project with this id, or the zero Project (empty key,
// not archived) when id is nil.
func OptionalProject(ctx context.Context, q *store.Queries, ws uuid.UUID, id *uuid.UUID) (store.Project, error) {
	if id == nil {
		return store.Project{}, nil
	}
	return q.GetProject(ctx, store.GetProjectParams{ID: *id, WorkspaceID: ws})
}

func chatProject(ctx context.Context, q *store.Queries, chat store.ChatSession) (store.Project, error) {
	return OptionalProject(ctx, q, chat.WorkspaceID, chat.ProjectID)
}

// endChatSegment completes the chat's current run, if any is still live, and
// detaches it so the next message starts a fresh segment. The caller holds the
// chat lock.
func endChatSegment(ctx context.Context, q *store.Queries, chat store.ChatSession, outcome string) (store.ChatSession, error) {
	if chat.CurrentRunID == nil {
		return chat, nil
	}
	run, err := q.LockRun(ctx, *chat.CurrentRunID)
	if err != nil {
		return chat, err
	}
	if !slices.Contains(Terminal, run.State) {
		if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "completed", Outcome: &outcome}); err != nil {
			return chat, err
		}
		if err = q.ExpireRunApprovals(ctx, run.ID); err != nil {
			return chat, err
		}
		if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "chat_segment_ended", "reason": "project_changed", "harness": chat.Harness}); err != nil {
			return chat, err
		}
		if err = RecomputeAgentStatus(ctx, q, run.AgentID); err != nil {
			return chat, err
		}
	}
	return q.SetChatRunState(ctx, store.SetChatRunStateParams{ID: chat.ID, State: chat.State})
}

// DeleteChat removes a chat and its messages. A running turn is cancelled
// first; the chat's runs stay for cost history.
func (s *Service) DeleteChat(ctx context.Context, ws, user, id uuid.UUID) error {
	return s.tx(ctx, func(q *store.Queries) error {
		chat, err := lockChat(ctx, q, ws, user, id)
		if err != nil {
			return err
		}
		return deleteChat(ctx, q, chat)
	})
}

// DeleteIdleTemporaryChat deletes a temporary chat that is still idle since
// before cutoff once its lock is held, so a message posted after the sweep
// listed it keeps the chat. Reports whether it was deleted.
func (s *Service) DeleteIdleTemporaryChat(ctx context.Context, ws, id uuid.UUID, cutoff time.Time) (bool, error) {
	deleted := false
	err := s.tx(ctx, func(q *store.Queries) error {
		chat, err := q.LockChatSession(ctx, store.LockChatSessionParams{ID: id, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		last := chat.CreatedAt
		if chat.LastMessageAt != nil {
			last = *chat.LastMessageAt
		}
		if !chat.Temporary || chat.State == "running" || !last.Before(cutoff) {
			return nil
		}
		deleted = true
		return deleteChat(ctx, q, chat)
	})
	return deleted, err
}

// deleteChat removes a locked chat; see DeleteChat.
func deleteChat(ctx context.Context, q *store.Queries, chat store.ChatSession) error {
	if err := q.ClearChatSuggestions(ctx, store.ClearChatSuggestionsParams{WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy}); err != nil {
		return err
	}
	if err := stopResearchForChat(ctx, q, chat.ID); err != nil {
		return err
	}
	if err := stopWorkflowsForChat(ctx, q, chat.ID); err != nil {
		return err
	}
	if err := q.KeepWorkflowImagesForSweep(ctx, chat.ID); err != nil {
		return err
	}
	if chat.CurrentRunID != nil {
		run, err := q.LockRun(ctx, *chat.CurrentRunID)
		if err != nil {
			return err
		}
		if !slices.Contains(Terminal, run.State) {
			if err = q.DropPendingTurns(ctx, run.ID); err != nil {
				return err
			}
			outcome := "The chat was deleted."
			if _, err = q.SetRunState(ctx, store.SetRunStateParams{ID: run.ID, State: "cancelled", Outcome: &outcome}); err != nil {
				return err
			}
			_ = q.EndOpenTurn(ctx, store.EndOpenTurnParams{RunID: run.ID, ExitCode: ptr[int32](143)})
			if err = q.ExpireRunApprovals(ctx, run.ID); err != nil {
				return err
			}
			if err = event(ctx, q, run.ID, "lifecycle", map[string]any{"state": "cancelled", "outcome": outcome}); err != nil {
				return err
			}
			if err = RecomputeAgentStatus(ctx, q, run.AgentID); err != nil {
				return err
			}
		}
	}
	return q.DeleteChatSession(ctx, chat.ID)
}
