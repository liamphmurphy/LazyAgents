// Pure logic for the chat screen (LAZYAGENTS-57): the live activity line, when a model switch asks first, and reply cost.
// Kept free of React so `npm run test:unit` covers it.

export type RunEventLike = { seq: number; type: string; at: string; payload: Record<string, unknown> };
export type ToolCall = { key: number; tool: string; input: unknown; done: boolean; output?: string; failed?: true };

const str = (v: unknown) => (v == null ? "" : typeof v === "string" ? v : JSON.stringify(v));

/**
 * Tool calls since `since` (the turn's user message) and up to `until` (the next one),
 * each marked done once its result arrives.
 */
export function toolCalls(events: RunEventLike[] | undefined, since?: string, until?: string): ToolCall[] {
  const t = since ? Date.parse(since) : 0;
  const hi = until ? Date.parse(until) : Infinity;
  const out: ToolCall[] = [];
  const open = new Map<string, number>();
  for (const e of events ?? []) {
    const at = Date.parse(e.at);
    if (at < t || at > hi) continue;
    const p = e.payload;
    if (e.type === "tool_call") {
      if (p.id) open.set(str(p.id), out.length);
      out.push({ key: e.seq, tool: str(p.tool), input: p.input, done: false });
    } else if (e.type === "tool_result" && p.id && open.has(str(p.id))) {
      const call = out[open.get(str(p.id))!];
      call.done = true;
      if (p.output) call.output = str(p.output);
      if (p.is_error) call.failed = true;
    }
  }
  return out;
}

/**
 * The running turn's opening line: the assistant's first text since `since`, but only
 * once a tool call follows it. A turn that answers straight away has no tool call, so
 * its text is the reply itself and isn't shown early.
 */
export function interimLine(events: RunEventLike[] | undefined, since?: string): string | undefined {
  const t = since ? Date.parse(since) : 0;
  let line: string | undefined;
  for (const e of events ?? []) {
    if (Date.parse(e.at) < t) continue;
    const p = e.payload;
    if (e.type === "message" && line === undefined && p.role === "assistant" && !p.kind) {
      const text = str(p.text).trim();
      if (text) line = text;
    } else if (e.type === "tool_call" && line !== undefined) {
      return clip(line, 280);
    }
  }
  return undefined;
}

type MessageLike = { id: string; role: string; created_at: string; run_id?: string; turn_seq?: number };

/** One turn's event window; `followup` marks a turn no person's message started (the image follow-up). */
export type TurnWindow = { since?: string; until?: string; followup: boolean };

/**
 * Each turn's last assistant reply gets the window of events between the turn's start
 * and the next one's, so a turn's memory notices show once, under its answer. A turn
 * starts at a user message, or right after an earlier turn's reply when the assistant
 * replies again on its own (a different run turn, e.g. reacting to generated images).
 */
export function turnWindows(list: MessageLike[] | undefined): Map<string, TurnWindow> {
  const out = new Map<string, TurnWindow>();
  let since: string | undefined;
  let reply: MessageLike | undefined;
  let followup = false;
  const close = (until?: string) => reply && out.set(reply.id, { since, until, followup });
  const turnOf = (m: MessageLike) => (m.turn_seq === undefined ? undefined : `${m.run_id}:${m.turn_seq}`);
  for (const m of list ?? []) {
    if (m.role === "user") {
      close(m.created_at);
      since = m.created_at;
      reply = undefined;
      followup = false;
    } else if (m.role === "assistant") {
      if (reply && turnOf(m) !== undefined && turnOf(reply) !== undefined && turnOf(m) !== turnOf(reply)) {
        close(reply.created_at);
        since = reply.created_at;
        followup = true;
      }
      reply = m;
    }
  }
  close(undefined);
  return out;
}

/**
 * Where the running turn's events start: the newest message. That is the person's
 * message for a turn they started, or the previous reply (and its notes) for a turn
 * the assistant takes on its own, so the earlier turn's opening line and tools aren't
 * shown again.
 */
export function runningSince(list: MessageLike[] | undefined): string | undefined {
  return list?.at(-1)?.created_at;
}

/** "mcp__lazyagents__read_page" (Claude) and "read_page" (Codex) are the same tool. */
export function bareTool(tool: string): string {
  const mcp = /^mcp__(.+?)__(.+)$/.exec(tool);
  if (mcp) return mcp[1] === "lazyagents" ? mcp[2] : `${mcp[1]}.${mcp[2]}`;
  return /^lazyagents[._]/.test(tool) ? tool.slice("lazyagents.".length) : tool;
}

/** A tool from one of the person's MCP connectors: harnesses name it mcp__slug__tool or slug.tool. */
export function connectorCall(tool: string): { name: string; tool: string } | null {
  const m = /^mcp__([a-z][a-z0-9-]*)__(.+)$/.exec(tool) ?? /^([a-z][a-z0-9-]*)\.([^.]+)$/.exec(tool);
  if (!m || m[1] === "lazyagents") return null;
  return { name: m[1].split("-").map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(" "), tool: m[2] };
}

/** What the live line says: a verb, then a [[ref]] to link or plain text. */
export type Activity = { verb: string; ref?: string; label?: string; text?: string };

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const clip = (s: string, n = 60) => (s.length > n ? `${s.slice(0, n - 1)}…` : s);

/** A page name without its folders, with the section: "Arch/ADR - X#risks" → "ADR - X › risks". */
export function linkLabel(ref: string): string {
  const i = ref.indexOf("#");
  const target = i < 0 ? ref : ref.slice(0, i);
  const anchor = i < 0 ? "" : ref.slice(i + 1);
  const name = target.split("/").filter(Boolean).pop() ?? "";
  if (!anchor) return name || target;
  return name ? `${name} › ${anchor}` : anchor;
}

/** The newest tool call in words: "Reading [[ADR - Project chat sessions]]", "Searching “ledger”". */
export function activityOf(call: { tool: string; input: unknown }): Activity {
  const o = (call.input && typeof call.input === "object" ? call.input : {}) as Record<string, unknown>;
  const s = (k: string) => (typeof o[k] === "string" ? (o[k] as string).trim() : "");
  const link = (verb: string, raw: string, fallback: string): Activity => {
    const ref = raw.replace(/^\[\[/, "").replace(/\]\]$/, "").split("|")[0].trim();
    return ref ? { verb, ref, label: clip(linkLabel(ref)) } : { verb: fallback };
  };
  switch (bareTool(call.tool)) {
    case "read_page":
      return link("Reading", s("ref"), "Reading a page");
    case "read_artifact": {
      const a = s("artifact");
      // An artifact id doesn't resolve as a link; ADR ids and titles do.
      if (!a || UUID.test(a)) return { verb: "Reading an artifact" };
      return link("Reading", s("section") ? `${a}#${s("section")}` : a, "Reading an artifact");
    }
    case "get_ticket":
      return link("Opening", s("ticket"), "Opening a ticket");
    case "get_backlinks":
      return link("Finding links to", s("ref"), "Finding links");
    case "search_knowledge":
      return s("q") ? { verb: "Searching", text: `“${clip(s("q"))}”` } : { verb: "Searching the knowledge base" };
    case "find_pattern":
      return s("q") ? { verb: "Looking up patterns for", text: `“${clip(s("q"))}”` } : { verb: "Looking up patterns" };
    case "WebSearch":
    case "web_search":
      return s("query") ? { verb: "Searching the web for", text: `“${clip(s("query"))}”` } : { verb: "Searching the web" };
    case "WebFetch": {
      let host = s("url");
      try {
        host = new URL(host).hostname.replace(/^www\./, "");
      } catch {}
      return host ? { verb: "Reading", text: clip(host) } : { verb: "Reading a web page" };
    }
    case "whoami":
      return { verb: "Checking its instructions" };
    case "shell":
    case "Bash":
      return { verb: "Running a command" };
    default: {
      const connector = connectorCall(call.tool);
      if (connector) return { verb: `Using ${connector.name}:`, text: connector.tool };
      return { verb: "Using", text: bareTool(call.tool) || "a tool" };
    }
  }
}

/**
 * Switching to another harness ends the session (the ADR's handoff), so it asks
 * first. A model change on the same harness keeps the session, an empty chat has
 * no session to lose, and a switch already confirmed for that harness doesn't ask again.
 */
export function needsSwitchConfirm(
  current: { harness: string },
  next: { harness: string },
  pending: { harness: string } | null | undefined,
  messageCount: number,
): boolean {
  return messageCount > 0 && next.harness !== current.harness && pending?.harness !== next.harness;
}

/** A reply's cost; small turns keep three decimals so they don't read $0.00. Unknown or unpriced is undefined, never free. */
export function replyCost(cost: number | undefined): string | undefined {
  if (cost === undefined || !Number.isFinite(cost) || cost <= 0) return undefined;
  if (cost < 0.001) return "< $0.001";
  return `$${cost.toFixed(cost < 1 ? 3 : 2)}`;
}

/** A chat's display title: a new conversation has none until the first turn names it. */
export function chatTitle(c: { title: string }): string {
  return c.title.trim() || "New conversation";
}

/** The most recently active chat: by its last message, else when it last changed. */
export function newestChat<T extends { last_message_at?: string | null; updated_at: string }>(chats: T[] | undefined): T | undefined {
  const t = (c: T) => Date.parse(c.last_message_at ?? c.updated_at);
  return chats?.reduce<T | undefined>((a, c) => (!a || t(c) > t(a) ? c : a), undefined);
}

// ---- the chat tab's status line: what the persona is doing, at a glance ----

/** The kind of work under way, which picks the status card's animated scene. */
export type ChatStatusKind =
  | "thinking"
  | "connector"
  | "wiki"
  | "tickets"
  | "reading-code"
  | "searching-code"
  | "writing-code"
  | "tests"
  | "build"
  | "install"
  | "terminal"
  | "github"
  | "commit"
  | "push"
  | "pull-request"
  | "repo-history"
  | "web"
  | "web-page"
  | "video"
  | "image"
  | "image-view"
  | "canvas"
  | "canvas-comment"
  | "runs"
  | "workflow"
  | "memory"
  | "memory-save";
export type ChatStatusInfo = { kind: ChatStatusKind; phrase: string };

const status = (kind: ChatStatusKind, phrase: string): ChatStatusInfo => ({ kind, phrase });

// Options may sit between `git` and its subcommand: "git -C /repo push".
const GIT = String.raw`\bgit(?:\s+-c\s+\S+|\s+--\S+)*\s+`;
const GIT_PUSH = new RegExp(`${GIT}push\\b`);
const GIT_COMMIT = new RegExp(`${GIT}commit\\b`);
const GIT_OTHER = new RegExp(`${GIT}(add|am|apply|blame|branch|checkout|cherry-pick|clone|diff|fetch|init|log|ls-files|merge|merge-base|pull|rebase|remote|reset|restore|rev-list|rev-parse|revert|rm|show|stash|status|submodule|switch|tag|worktree)\\b`);

/**
 * A shell command as a kind of work, never the command itself: the chat tab says
 * "running tests", the Activity tab keeps the exact command.
 */
function shellStatus(command: string): ChatStatusInfo {
  // Codex runs everything through a login shell: /bin/zsh -lc 'cat x'.
  const c = command.toLowerCase().replace(/^(\S*\/)?(ba|z)?sh\s+-l?c\s+['"]?/, "");
  if (/\.lazyagents\/outbox/.test(c)) return status("image", "saving the image");
  if (GIT_PUSH.test(c)) return status("push", "pushing its branch");
  if (GIT_COMMIT.test(c)) return status("commit", "committing changes");
  if (/\bgh\s+pr\b/.test(c)) return status("pull-request", "preparing a pull request");
  if (/\bgh\s+\S/.test(c)) return status("github", "working on GitHub");
  if (/\b(go\s+test|npm\s+(run\s+)?test|pnpm\s+(run\s+)?test|yarn\s+test|pytest|jest|vitest|cargo\s+test|make\s+test|rspec|playwright\s+test|mvn\s+test|gradle\w*\s+test)\b/.test(c)) return status("tests", "running tests");
  if (/\b(npm\s+(ci|install|i)|pnpm\s+install|yarn(\s+install)?\s*$|go\s+mod\s+(download|tidy)|pip\s+install|bundle\s+install|cargo\s+fetch)\b/.test(c)) return status("install", "installing dependencies");
  if (/\b(go\s+(build|vet)|npm\s+run\s+(build|lint|typecheck)|tsc\b|cargo\s+(build|check|clippy)|make\b|eslint|gofmt)\b/.test(c)) return status("build", "checking the build");
  if (GIT_OTHER.test(c)) return status("repo-history", "looking over the repository");
  if (/^\s*(find|rg|grep|fd)\b|\|\s*(rg|grep)\b/.test(c)) return status("searching-code", "searching the code");
  if (/^\s*(cat|less|head|tail|ls|tree|sed\s+-n|wc|awk)\b|\|\s*(head|tail)\b/.test(c)) return status("reading-code", "reading the code");
  return status("terminal", "working in the terminal");
}

/** An MCP GitHub tool, from any server name: "github.create_pull_request", "plugin_github_github.get_issue". */
function githubToolStatus(tool: string): ChatStatusInfo | undefined {
  if (!/(^|[._-])github[._-]/i.test(tool)) return undefined;
  if (/(create|update|merge)\w*pull_?request/i.test(tool)) return status("pull-request", "preparing a pull request");
  return status("github", "working on GitHub");
}

/** A rich block being written into the reply, reported by the harness: no tool runs, but it can take a while. */
const REPLY_BLOCK = "reply_block";
const REPLY_BLOCK_STATUS: Record<string, ChatStatusInfo> = {
  app: status("canvas", "building an interactive app"),
  "svg-image": status("image", "drawing an image"),
  "svg-steps": status("image", "drawing a step-through"),
  chart: status("canvas", "drawing a chart"),
  mermaid: status("canvas", "drawing a diagram"),
  choices: status("canvas", "laying out your options"),
  "workflow-run": status("workflow", "lining up a workflow"),
  recommend: status("web", "picking something for you to look at"),
};

/** The running turn's tool calls and the rich blocks written between them, in order, for the chat tab. */
export function chatCalls(events: RunEventLike[] | undefined, since?: string): ToolCall[] {
  const t = since ? Date.parse(since) : 0;
  const blocks = (events ?? [])
    .filter((e) => e.type === "lifecycle" && e.payload.state === REPLY_BLOCK && Date.parse(e.at) >= t)
    .map((e): ToolCall => ({ key: e.seq, tool: REPLY_BLOCK, input: { kind: e.payload.kind }, done: true }));
  return [...toolCalls(events, since), ...blocks].sort((a, b) => a.key - b.key);
}

/**
 * One tool call as a kind of work and a short, high-level phrase for the chat tab
 * ("reading the code", "looking through the wiki"): no queries, paths, refs or commands,
 * which stay in the Activity tab. The phrase completes "<Persona> is …".
 */
export function chatStatusOf(call: { tool: string; input: unknown }): ChatStatusInfo {
  const o = (call.input && typeof call.input === "object" ? call.input : {}) as Record<string, unknown>;
  const tool = bareTool(call.tool);
  switch (tool) {
    case "read_page":
    case "read_artifact":
    case "get_backlinks":
    case "search_knowledge":
    case "find_pattern":
      return status("wiki", "looking through the wiki");
    case "get_ticket":
    case "list_epics":
      return status("tickets", "checking tickets");
    case "create_ticket":
    case "create_epic":
      return status("tickets", "drafting a ticket");
    case "list_runs":
    case "get_run":
      return status("runs", "checking on runs");
    case "search_memory":
    case "read_memory":
      return status("memory", "recalling what it knows about you");
    case "save_memory":
    case "update_memory":
      return status("memory-save", "making a note");
    case "forget_memory":
      return status("memory", "forgetting that");
    case "set_chat_title":
      return status("thinking", "naming the conversation");
    case "WebSearch":
    case "web_search":
    case "websearch":
      return status("web", "searching the web");
    case "WebFetch":
    case "webfetch":
      return status("web-page", "reading a page");
    case "list_saved_links":
      return status("web", "checking your saved links");
    case "propose_persona":
      return status("memory", "sketching a new persona");
    case "create_space":
      return status("thinking", "setting up a space");
    case "create_scheduled_task":
      return status("workflow", "scheduling a task");
    case "create_learning_module":
      return status("wiki", "starting a learning module");
    case "submit_research_report":
      return status("wiki", "writing up its research");
    case "get_video_transcript":
      return status("video", "watching a video");
    case "canvas_list":
    case "canvas_read":
      return status("canvas", "looking at a canvas");
    case "canvas_create":
      return status("canvas", "drafting a canvas");
    case "canvas_patch":
      return status("canvas", "revising a canvas");
    case "canvas_comment":
    case "canvas_reply":
      return status("canvas-comment", "commenting on a canvas");
    case "list_workflows":
    case "read_workflow":
      return status("workflow", "looking over a workflow");
    case "propose_workflow_edit":
      return status("workflow", "sketching a workflow");
    case "image_generation":
    case "image_gen":
    case "generate_image":
      return status("image", "creating an image");
    case "Read":
    case "read":
    case "read_file":
      // The image follow-up opens the generated images it is about to react to.
      if (typeof o.file_path === "string" && o.file_path.includes(".lazyagents/attachments/generated/")) return status("image-view", "looking at the image");
      return status("reading-code", "reading the code");
    case "Glob":
    case "glob":
    case "Grep":
    case "grep":
      return status("searching-code", "searching the code");
    case "LS":
    case "list":
    case "list_dir":
      return status("reading-code", "reading the code");
    case "Edit":
    case "edit":
    case "MultiEdit":
    case "Write":
    case "write":
    case "NotebookEdit":
    case "apply_patch":
    case "patch":
      return status("writing-code", "writing code");
    case REPLY_BLOCK:
      return REPLY_BLOCK_STATUS[str(o.kind)] ?? status("thinking", "working on it");
    case "TodoWrite":
    case "todowrite":
    case "TaskCreate":
    case "TaskUpdate":
      return status("thinking", "planning its next steps");
    case "shell":
    case "Bash":
    case "bash":
    case "exec_command":
    case "shell_command":
      return shellStatus(typeof o.command === "string" ? o.command : typeof o.cmd === "string" ? o.cmd : "");
    default: {
      const connector = connectorCall(call.tool);
      if (connector) return status("connector", `using ${connector.name}`);
      return githubToolStatus(tool) ?? status("thinking", "working on it");
    }
  }
}

/** The chat tab's status for a running turn: the newest call's, or "thinking" before any tool runs. */
export function chatStatus(calls: { tool: string; input: unknown }[]): ChatStatusInfo {
  return calls.length ? chatStatusOf(calls[calls.length - 1]) : status("thinking", "thinking");
}

// ---- the chat tab's step rail: the turn's plan, or its work so far ----

/** A step's place in the turn. A finished step is "failed" when its last call errored at work where an error means failure. */
export type ChatStepState = "planned" | "active" | "done" | "failed";
/** `kind` picks the step's glyph: the kind of its newest call, absent until a planned step's work starts. */
export type ChatStep = { key: string; title: string; state: ChatStepState; kind?: ChatStatusKind; added?: true; detail?: string };

type PlanState = "planned" | "active" | "done";
type PlanItem = { id: string; title: string; state: PlanState };

const planState = (v: unknown): PlanState => (v === "completed" ? "done" : v === "in_progress" ? "active" : "planned");
const records = (list: unknown) => (Array.isArray(list) ? (list as Record<string, unknown>[]) : []);
const planItems = (list: unknown): PlanItem[] =>
  records(list)
    .filter((item) => item.content && item.status !== "cancelled")
    .map((item, i) => ({ id: String(i), title: str(item.content), state: planState(item.status) }));

/**
 * Every version of the plan in these calls, oldest first: TodoWrite sends the whole list, TaskCreate and TaskUpdate change one task.
 * `call` is the index of the call that wrote each version.
 */
type PlanVersion = { items: PlanItem[]; call: number };
function planVersions(calls: ToolCall[]): PlanVersion[] {
  const versions: PlanVersion[] = [];
  let tasks: PlanItem[] = [];
  for (const [at, call] of calls.entries()) {
    const o = (call.input && typeof call.input === "object" ? call.input : {}) as Record<string, unknown>;
    switch (bareTool(call.tool)) {
      case "TodoWrite":
      case "todowrite":
        versions.push({ items: planItems(o.todos), call: at });
        break;
      case "TaskCreate":
        tasks = [...tasks, { id: /#(\d+)/.exec(call.output ?? "")?.[1] ?? String(tasks.length + 1), title: str(o.subject), state: "planned" }];
        versions.push({ items: tasks, call: at });
        break;
      case "TaskUpdate": {
        const id = str(o.taskId);
        if (!tasks.some((t) => t.id === id)) break;
        tasks = o.status === "deleted"
          ? tasks.filter((t) => t.id !== id)
          : tasks.map((t) => (t.id === id ? { ...t, title: o.subject ? str(o.subject) : t.title, state: o.status ? planState(o.status) : t.state } : t));
        versions.push({ items: tasks, call: at });
        break;
      }
    }
  }
  return versions;
}

/** Kinds where an errored call means the work failed. Elsewhere an error is routine: a search with no match, an edit retried. */
const FAILABLE = new Set<ChatStatusKind>(["tests", "build", "install", "commit", "push", "pull-request", "github", "terminal", "connector", "image"]);

const failedAt = (call: ToolCall, kind: ChatStatusKind) => !!call.failed && FAILABLE.has(kind);

/**
 * Each plan step's kind and outcome, from the calls made while it was the step under way:
 * the newest call's kind, and whether that call failed.
 */
function planWork(calls: ToolCall[], versions: PlanVersion[]): Map<string, { kind: ChatStatusKind; failed: boolean }> {
  const work = new Map<string, { kind: ChatStatusKind; failed: boolean }>();
  let v = -1;
  for (const [i, call] of calls.entries()) {
    while (v + 1 < versions.length && versions[v + 1].call <= i) v++;
    if (v < 0 || BOOKKEEPING.has(bareTool(call.tool)) || call.tool === REPLY_BLOCK) continue;
    const active = versions[v].items.find((s) => s.state === "active");
    if (!active) continue;
    const { kind } = chatStatusOf(call);
    work.set(active.title, { kind, failed: failedAt(call, kind) });
  }
  return work;
}

/** Calls that keep the turn running but aren't work to show: its plan, tool loading, the chat's title. */
const BOOKKEEPING = new Set(["TodoWrite", "todowrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "ToolSearch", "Skill", "set_chat_title", "whoami"]);

const basename = (p: string) => p.split("/").filter(Boolean).pop() ?? p;

/**
 * What a step is doing right now, safe for the chat tab: a file's name but not its path,
 * search terms, page names and sites. A shell command stays its kind of work ("Running tests"),
 * since commands are long and can carry secrets. Memory stays general because it's private.
 * Tools that take only an id use `names`, what earlier calls in the turn said it's called.
 */
export function stepDetail(call: { tool: string; input: unknown }, names: Map<string, string> = new Map()): string {
  const o = (call.input && typeof call.input === "object" ? call.input : {}) as Record<string, unknown>;
  const { kind, phrase } = chatStatusOf(call);
  const general = phrase[0].toUpperCase() + phrase.slice(1);
  const file = basename(str(o.file_path ?? o.notebook_path ?? o.path));
  const quoted = `“${clip(str(o.pattern))}”`;
  const named = (verb: string, name: unknown, fallback?: unknown) => (name ? `${verb} “${clip(str(name))}”` : fallback ? `${verb}: ${clip(str(fallback))}` : general);
  const known = (id: unknown) => names.get(str(id));
  switch (bareTool(call.tool)) {
    case "Read":
    case "read_file":
      return file && kind !== "image-view" ? `Reading ${file}` : general;
    case "Edit":
    case "MultiEdit":
    case "NotebookEdit":
      return file ? `Editing ${file}` : general;
    case "Write":
      return file ? `Writing ${file}` : general;
    case "Grep":
      return o.pattern ? `Searching ${quoted}` : general;
    case "Glob":
      return o.pattern ? `Finding ${quoted}` : general;
    case "LS":
    case "list_dir":
      return file ? `Listing ${file}` : general;
    case "apply_patch": {
      // Codex reports each change as "<kind> <path>".
      const files = Array.isArray(o.files) ? o.files.map((f) => basename(str(f).split(" ").slice(1).join(" "))) : [];
      return files.length ? `Editing ${files[0]}${files.length > 1 ? ` and ${files.length - 1} more` : ""}` : general;
    }
    case "canvas_create":
      return named("Drafting", o.title);
    case "canvas_read":
      return named("Looking at", known(o.canvas_id));
    case "canvas_patch":
      return named("Revising", known(o.canvas_id), o.note);
    case "canvas_comment":
    case "canvas_reply":
      return named("Commenting on", known(o.canvas_id));
    case "read_workflow":
      return named("Looking over", known(o.workflow_id));
    case "propose_workflow_edit":
      return named("Sketching", o.name || known(o.workflow_id), o.summary);
    case "create_ticket":
      return named("Drafting", o.title);
    case "create_epic":
      return named("Drafting", o.name);
    case "generate_image":
    case "image_generation":
    case "image_gen":
      return named("Creating", o.prompt);
    case "propose_persona":
      return named("Sketching", o.name);
    case "create_space":
      return named("Setting up", o.name);
    case "create_scheduled_task":
      return named("Scheduling", o.name);
    case "create_learning_module":
      return named("Starting a module on", o.topic);
    case "get_run":
      return known(o.id) ? `Checking ${known(o.id)}` : general;
    case "list_runs":
      return o.ticket || o.agent ? `Checking runs of ${clip(str(o.ticket || o.agent))}` : general;
    case "get_video_transcript":
      return typeof o.from_seconds === "number" && o.from_seconds > 0 ? `Watching from ${Math.floor(o.from_seconds / 60)}:${String(Math.floor(o.from_seconds % 60)).padStart(2, "0")}` : general;
  }
  if (kind === "github" || kind === "pull-request") {
    const number = o.pull_number ?? o.pullNumber ?? o.issue_number ?? o.number;
    if (o.title) return named("Working on", o.title);
    if (o.repo) return `Working on ${clip(`${o.owner ? `${str(o.owner)}/` : ""}${str(o.repo)}${number ? `#${str(number)}` : ""}`)}`;
    return general;
  }
  const a = activityOf(call);
  return a.verb === "Using" || a.verb === "Running a command" ? general : [a.verb, a.label, a.text].filter(Boolean).join(" ");
}

/** A tool result as JSON: plain, or the MCP envelope Codex passes through. Undefined when it was clipped or isn't JSON. */
function resultJSON(output?: string): Record<string, unknown> | undefined {
  try {
    const v = JSON.parse(output ?? "");
    const text = v?.content?.[0]?.text;
    return typeof text === "string" ? JSON.parse(text) : v;
  } catch {
    return undefined;
  }
}

/** The names the turn has seen for canvases, workflows and runs, by id. */
function knownNames(calls: ToolCall[]): Map<string, string> {
  const names = new Map<string, string>();
  const note = (id: unknown, name: unknown) => {
    if (id && name) names.set(str(id), str(name));
  };
  for (const call of calls) {
    const out = resultJSON(call.output);
    if (!out) continue;
    const input = (call.input ?? {}) as Record<string, unknown>;
    switch (bareTool(call.tool)) {
      case "canvas_create":
        note(out.canvas_id, input.title);
        break;
      case "canvas_read":
        note(out.canvas_id, out.title);
        break;
      case "canvas_list":
        for (const c of records(out.canvases)) note(c.canvas_id, c.title);
        break;
      case "list_workflows":
        for (const w of records(out.workflows)) note(w.id, w.name);
        break;
      case "read_workflow":
        note(out.id, out.name);
        break;
      case "propose_workflow_edit":
        note(out.workflow_id, input.name);
        break;
      case "list_runs":
        for (const r of records(out.runs)) note(r.id, r.ticket);
        break;
      case "get_run": {
        const run = out.run as Record<string, unknown> | undefined;
        note(run?.id, run?.ticket);
        break;
      }
    }
  }
  return names;
}

/** Puts what's happening now on the step under way, unless it would only repeat the title. */
function withDetail(steps: ChatStep[], calls: ToolCall[]): ChatStep[] {
  const newest = calls.filter((c) => !BOOKKEEPING.has(bareTool(c.tool))).pop();
  const active = steps.find((s) => s.state === "active");
  if (!newest || !active || newest.tool === REPLY_BLOCK) return steps;
  const detail = stepDetail(newest, knownNames(calls));
  if (detail !== active.title) active.detail = detail;
  return steps;
}

/**
 * The turn's steps. The newest plan when the assistant wrote one, marking steps added after
 * work began. Otherwise its consecutive tool calls with the same description as one step, newest active, in the same
 * high-level wording as the status line.
 */
export function chatSteps(calls: ToolCall[]): { steps: ChatStep[]; planned: boolean } {
  const versions = planVersions(calls);
  const latest = versions[versions.length - 1]?.items;
  if (latest?.length) {
    const baseline = versions.find((v) => v.items.some((s) => s.state !== "planned"))?.items;
    const original = new Set(baseline?.map((s) => s.title) ?? latest.map((s) => s.title));
    const work = planWork(calls, versions);
    const seen = new Map<string, number>();
    const steps = latest.map((s): ChatStep => {
      // By title, not position, so a step added mid-list doesn't remount the ones after it.
      const n = seen.get(s.title) ?? 0;
      seen.set(s.title, n + 1);
      const w = s.state === "planned" ? undefined : work.get(s.title);
      return {
        key: `${s.title}#${n}`,
        title: s.title,
        state: s.state === "done" && w?.failed ? "failed" : s.state,
        ...(w ? { kind: w.kind } : {}),
        ...(original.has(s.title) ? {} : { added: true as const }),
      };
    });
    // A plan doesn't list the rich blocks in the reply, so they follow it.
    const newest = calls[calls.length - 1];
    for (const call of calls) {
      if (call.tool !== REPLY_BLOCK) continue;
      const { kind, phrase } = chatStatusOf(call);
      steps.push({ key: String(call.key), title: phrase[0].toUpperCase() + phrase.slice(1), state: call === newest ? "active" : "done", kind });
    }
    return { steps: withDetail(steps, calls), planned: true };
  }
  const steps: ChatStep[] = [];
  let last: string | undefined;
  for (const call of calls) {
    if (BOOKKEEPING.has(bareTool(call.tool))) continue;
    const { kind, phrase } = chatStatusOf(call);
    const failed = failedAt(call, kind);
    if (phrase === last) {
      // The group's newest call decides: a test run that failed, then passed, is done.
      steps[steps.length - 1].state = failed ? "failed" : "done";
      continue;
    }
    last = phrase;
    steps.push({ key: String(call.key), title: phrase[0].toUpperCase() + phrase.slice(1), state: failed ? "failed" : "done", kind });
  }
  if (steps.length) steps[steps.length - 1].state = "active";
  return { steps: withDetail(steps, calls), planned: false };
}

/**
 * Keeps the rail short on long turns: all but the newest two of more than three finished steps fold away, and at most four upcoming steps show.
 * A failed step never folds, so a failure stays in sight.
 */
export function foldSteps(steps: ChatStep[]): { earlier: number; shown: ChatStep[]; later: number } {
  const done = steps.filter((s) => s.state === "done");
  const hidden = new Set(done.length > 3 ? done.slice(0, -2) : []);
  const upcoming = steps.filter((s) => s.state === "planned");
  for (const s of upcoming.slice(4)) hidden.add(s);
  return { earlier: done.length > 3 ? done.length - 2 : 0, shown: steps.filter((s) => !hidden.has(s)), later: Math.max(0, upcoming.length - 4) };
}
