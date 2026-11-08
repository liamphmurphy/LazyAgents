import type { Page, Route } from "@playwright/test";
import type { components } from "../lib/api/schema";

/**
 * In-memory stand-in for the chat REST contract (api/openapi.yaml, tag `chats`)
 * plus the fake harness: an assistant reply that echoes the prompt, tagged with
 * the selected harness/model. Swap `installChatApi` for the real stack once
 * LAZYAGENTS-11 lands; the spec only talks to the UI.
 *
 * A message starting with "slow:" keeps the turn running until it is cancelled
 * or interrupted: a message sent while a turn runs stops it, and the next reply
 * echoes "<new> (interrupting: <previous>)", like the server's merged turn;
 * its run reports one tool call (read_page Eng/Plan) for the live activity line.
 * Replies carry token usage and cost and a run_id whose events (GET /runs/{id}) hold the turn's tool calls.
 *
 * Assistant memory (LAZYAGENTS-71): an in-memory store behind /memories. A message
 * "remember: <title>" makes the turn call save_memory (creating the entry, sourced
 * from the chat) and "forget: <title>" calls forget_memory (deleting it); both put a
 * successful tool_result into the turn's run events, between the user message and the reply.
 *
 * Assistant personas (LAZYAGENTS-86): `opts.personas` seeds GET /personas (none by
 * default, so the built-in Assistant is the default). GET /chats honours persona_id
 * (omitted: the default persona), POST /chats takes it (omitted: the default), and
 * `updatePersona` edits one and pushes `persona.updated` like the real stream.
 * `chatPersonaFilters` records the persona_id each GET /chats asked for.
 */

type Harness = "claude-code" | "codex" | "opencode";
type State = "idle" | "running" | "failed";
type Effort = "low" | "medium" | "high" | "xhigh" | "max";
type Chat = {
  id: string; project: string; title: string; agent_id: string; harness: Harness; model: string; state: State;
  error?: string; current_run_id?: string; archived: boolean; revision: number; message_count: number;
  created_at: string; updated_at: string; last_message_at?: string; persona_id?: string; repo_url?: string; work_branch?: string;
  effort?: Effort;
};
type Message = {
  id: string; seq: number; role: "user" | "assistant" | "system"; body_md: string;
  harness?: Harness; model?: string; created_at: string;
  context_refs?: unknown[]; citations?: unknown[]; run_id?: string; code_changes?: unknown[];
};
export type MockMemory = {
  id: string; title: string; summary: string; body: string; tags: string[]; type: string;
  source: string; source_chat_id?: string; created_at: string; updated_at: string;
};

// What the [[ picker can offer (api: GET /knowledge/suggest). OTHER-9 lives in another project.
const PLAN = { kind: "artifact", id: "11111111-1111-4111-8111-111111111111", title: "Plan", artifact_type: "note", path: "Eng/Plan", href: "/wiki/Eng/Plan" };
const REPORT = { kind: "artifact", id: "22222222-2222-4222-8222-222222222222", title: "Q3 report", artifact_type: "report", href: "/artifacts/22222222-2222-4222-8222-222222222222" };
const TICKET = { kind: "ticket", id: "33333333-3333-4333-8333-333333333333", title: "Fix payouts", display_id: "WEB-3", href: "/tickets/WEB-3", status: "todo" };
const OTHER = { kind: "ticket", id: "44444444-4444-4444-8444-444444444444", title: "Elsewhere", display_id: "OTHER-9", href: "/tickets/OTHER-9", status: "todo" };
const TARGETS: Array<{ title: string; path?: string; display_id?: string }> = [PLAN, REPORT, TICKET, OTHER];

// Bootstrap agents: two main agents a chat can answer as, and a sub agent that can't.
export const AGENTS = [
  { id: "aaaaaaaa-0000-4000-8000-000000000001", name: "Atlas", handle: "atlas", kind: "main", color_index: 0, role: "Tech lead", specialty: "Plans and reviews", model: "opus-5.5", status: "idle" },
  { id: "aaaaaaaa-0000-4000-8000-000000000002", name: "Juno", handle: "juno", kind: "main", color_index: 3, role: "Mastermind", specialty: "Product and UX", model: "opus-5.5", status: "idle" },
  { id: "aaaaaaaa-0000-4000-8000-000000000003", name: "Helper", handle: "helper", kind: "sub", color_index: 5, role: "Implementer", specialty: "Code", model: "sonnet-5.5", status: "idle" },
];
const USAGE = { input_tokens: 48200, output_tokens: 1300, cached_input_tokens: 41000, uncached_input_tokens: 7200, unreported_input_tokens: 0, cost_usd: 0.0421 };

export type Posted = { chat: string; body: Record<string, unknown> };
export type MockPersona = {
  id: string; name: string; tone: string; color: string; avatar_url?: string; custom_instructions: string;
  capabilities: { code: boolean; images: boolean; personas?: boolean; svg?: boolean }; repos: string[];
  is_default: boolean; chat_count: number; last_used_at?: string; revision: number; created_at: string; updated_at: string;
};
export type MockProposal = {
  id: string; chat_id: string; name: string; tone: string; color: string; custom_instructions: string; reason: string;
  state: "pending" | "approved" | "declined"; proposed_by?: { id: string; name: string; color: string }; created_at: string;
};
export type MockOptions = { codexAvailable?: boolean; opencodeAvailable?: boolean; replyDelayMs?: number; personas?: MockPersona[] };

/** A persona fixture for the persona specs. */
export function persona(i: number, over: Partial<MockPersona> = {}): MockPersona {
  const t = `2026-11-0${Math.min(i, 9)}T09:00:00.000Z`;
  return {
    id: `77777777-7777-4777-8777-${String(i).padStart(12, "0")}`,
    name: `Persona ${i}`, tone: "neutral", color: "#8b8fff", custom_instructions: "", capabilities: { code: false, images: false }, repos: [], is_default: false,
    chat_count: 0, revision: 1, created_at: t, updated_at: t, ...over,
  };
}

export const catalog = (codexAvailable: boolean, opencodeAvailable = true): components["schemas"]["ChatHarnessModels"][] => [
  {
    harness: "claude-code", label: "Claude Code", available: true,
    models: [
      { id: "opus-5.5", label: "Opus 5.5", default: true, efforts: ["low", "medium", "high", "xhigh", "max"] },
      { id: "sonnet-5.5", label: "Sonnet 5.5", default: false, efforts: ["low", "medium", "high", "xhigh", "max"] },
    ],
  },
  {
    harness: "codex", label: "Codex", available: codexAvailable,
    ...(codexAvailable ? {} : { unavailable_reason: "No harness:openai secret" }),
    models: [{ id: "gpt-5-codex", label: "gpt-5-codex", default: true, efforts: ["low", "medium", "high"], default_effort: "medium" }],
  },
  {
    harness: "opencode", label: "OpenCode", available: opencodeAvailable,
    ...(opencodeAvailable ? {} : { unavailable_reason: "OpenCode is not installed" }),
    models: [
      { id: "anthropic/claude-sonnet-4-5", label: "anthropic/claude-sonnet-4-5", default: true, efforts: [] },
      { id: "openai/gpt-5", label: "openai/gpt-5", default: false, efforts: [] },
    ],
  },
];

const err = (status: number, code: string, message: string, field?: string) => ({
  status,
  body: { code, message, ...(field ? { field } : {}) },
});

export function installChatApi(page: Page, opts: MockOptions = {}) {
  const codexAvailable = opts.codexAvailable ?? true;
  const opencodeAvailable = opts.opencodeAvailable ?? true;
  const delay = opts.replyDelayMs ?? 300;
  const chats: Chat[] = [];
  const messages = new Map<string, Message[]>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  // The running turn's prompt per chat, which an interruption folds in.
  const prompts = new Map<string, string>();
  const runEvents = new Map<string, Array<{ seq: number; type: string; at: string; payload: Record<string, unknown> }>>();
  const unhandled: string[] = [];
  const posted: Posted[] = [];
  const created: Array<Record<string, unknown>> = [];
  const artifactForms: string[] = [];
  const memories: MockMemory[] = [];
  const memoryRequests: string[] = [];
  const memoryPatches: Array<Record<string, unknown>> = [];
  const memoryImports: Array<Record<string, unknown>> = [];
  const wikiPages: Array<{ path: string; body_md: string; project_key?: string }> = [];
  // Assistant personas (ADR: Assistant personas): private to the caller, at most one default.
  const personas: MockPersona[] = [...(opts.personas ?? [])];
  // Personas the chat's personas proposed (several personas in one chat).
  const proposals: MockProposal[] = [];
  const decisions: Array<{ id: string; decision: string }> = [];
  const defaultPersona = () => personas.find((p) => p.is_default);
  const personaRequests: string[] = [];
  // The persona_id each GET /chats asked for ("" when omitted, i.e. the default).
  const chatPersonaFilters: string[] = [];
  let n = 0;
  const uuid = () => `00000000-0000-4000-8000-${String(++n).padStart(12, "0")}`;
  const now = () => new Date().toISOString();
  const label = (h: Harness, m: string) => {
    const g = catalog(true, true).find((x) => x.harness === h)!;
    return `${g.label} · ${g.models.find((x) => x.id === m)?.label ?? m}`;
  };
  const add = (c: Chat, role: Message["role"], body: string, extra: Partial<Message> = {}) => {
    const list = messages.get(c.id)!;
    const m: Message = { id: uuid(), seq: list.length + 1, role, body_md: body, created_at: now(), ...extra };
    list.push(m);
    c.message_count = list.length;
    c.last_message_at = c.updated_at = m.created_at;
    return m;
  };
  const touch = (c: Chat) => {
    c.revision++;
    c.updated_at = now();
  };

  const emit = (c: Chat) =>
    page
      .evaluate((id) => {
        const data = JSON.stringify({ type: "chat.updated", ids: [id] });
        for (const s of (window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> }).__sse) s.onmessage?.({ data });
      }, c.id)
      .catch(() => undefined); // page may be navigating; the next fetch catches up

  function memoryEvents(c: Chat, prompt: string) {
    const save = /^remember:\s*(.+)$/.exec(prompt);
    const forget = /^forget:\s*(.+)$/.exec(prompt);
    if (!save && !forget) return;
    const runId = c.current_run_id!;
    const events = runEvents.get(runId) ?? [];
    // Strictly after the user message, so the turn window (since < at) holds.
    const lastUser = messages.get(c.id)!.findLast((x) => x.role === "user");
    const at = new Date(Date.parse(lastUser?.created_at ?? now()) + 1).toISOString();
    const tool = (name: string, output: unknown) => {
      const id = `mem-${events.length + 1}`;
      events.push({ seq: events.length + 1, type: "tool_call", at, payload: { id, tool: `mcp__lazyagents__${name}`, input: {} } });
      events.push({ seq: events.length + 1, type: "tool_result", at, payload: { id, tool: `mcp__lazyagents__${name}`, output: JSON.stringify(output) } });
    };
    if (save) {
      const title = save[1].trim();
      const t = now();
      const m: MockMemory = {
        id: uuid(), title, summary: `About ${title}`, body: `Details about ${title}`, tags: ["chat"], type: "fact",
        source: `chat ${c.id}`, source_chat_id: c.id, created_at: t, updated_at: t,
      };
      memories.unshift(m);
      tool("save_memory", { id: m.id, action: "created", title });
    } else if (forget) {
      const title = forget[1].trim();
      const i = memories.findIndex((x) => x.title === title);
      if (i < 0) return;
      const [gone] = memories.splice(i, 1);
      tool("forget_memory", { forgotten: gone.id, title });
    }
    runEvents.set(runId, events);
  }

  // "web:" runs a search with three results; "webnone:" gets the same search and uses none of it.
  function webEvents(c: Chat) {
    const runId = c.current_run_id!;
    const events = runEvents.get(runId) ?? [];
    const lastUser = messages.get(c.id)!.findLast((x) => x.role === "user");
    const at = new Date(Date.parse(lastUser?.created_at ?? now()) + 1).toISOString();
    const links = [{ title: "Pricing page", url: "https://www.example.com/pricing" }, { title: "Latency notes", url: "https://b.example.org/latency" }, { title: "Unused forum", url: "https://forum.example.net/t/1" }];
    const id = `web-${events.length + 1}`;
    events.push({ seq: events.length + 1, type: "tool_call", at, payload: { id, tool: "WebSearch", input: { query: "tts pricing" } } });
    events.push({ seq: events.length + 1, type: "tool_result", at, payload: { id, tool: "WebSearch", output: `Links: ${JSON.stringify(links)}` } });
    runEvents.set(runId, events);
  }

  function finishTurn(c: Chat, prompt: string) {
    c.state = "idle";
    memoryEvents(c, prompt);
    if (prompt.startsWith("web:") || prompt.startsWith("webnone:")) webEvents(c);
    if (prompt.startsWith("web:") || prompt.startsWith("webnone:")) {
      const body = prompt.startsWith("web:") ? "The answer.\n\nSources:\n- https://example.com/pricing\n- https://b.example.org/latency" : "I found nothing useful.";
      add(c, "assistant", body, { harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE, ...(prompt.startsWith("web:") ? { citations: [{ target: TICKET, title: TICKET.title }] } : {}) });
      void emit(c);
      return;
    }
    // "cite:" prompts get citations (one duplicate, one versioned section, one ticket).
    const citations = prompt.startsWith("cite:")
      ? [
          { target: REPORT, artifact_id: REPORT.id, title: REPORT.title, version_id: "55555555-5555-4555-8555-555555555555", version: "v2", section: "risks" },
          { target: REPORT, artifact_id: REPORT.id, title: REPORT.title, version_id: "55555555-5555-4555-8555-555555555555", version: "v2", section: "risks" },
          { target: PLAN, artifact_id: PLAN.id, title: PLAN.title, path: PLAN.path },
          { target: TICKET, title: TICKET.title },
        ]
      : undefined;
    // "code:" replies like a coding persona: changed files (two hunks, an added file, a binary) and a long fence.
    if (prompt.startsWith("code:")) {
      const long = Array.from({ length: 25 }, (_, i) => `  const v${i} = ${i};`).join("\n");
      add(c, "assistant", `Done.\n\n\`\`\`ts\nfunction run() {\n${long}\n}\n\`\`\``, {
        harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE,
        code_changes: [
          {
            path: "src/app.ts", status: "modified", additions: 2, deletions: 1, binary: false, truncated: false,
            patch: "@@ -1,3 +1,4 @@\n import a from \"a\";\n-const x = 1;\n+const x = 2;\n+const y = 3;\n export {};\n@@ -20,2 +21,2 @@\n function f() {\n-  return 1;\n+  return 2;\n }\n",
          },
          { path: "cmd/main.go", status: "added", additions: 3, deletions: 0, binary: false, truncated: false, patch: "@@ -0,0 +1,3 @@\n+package main\n+\n+func main() {}\n" },
          { path: "assets/logo.png", status: "modified", additions: 0, deletions: 0, binary: true, truncated: false },
        ],
      });
      void emit(c);
      return;
    }
    // "app:" replies with an interactive app: a counter, plus a fetch the sandbox must block.
    if (prompt.startsWith("app:")) {
      const app = `<!DOCTYPE html>\n<button id="b">Count: 0</button><p id="net">pending</p>\n<script>let n=0;b.onclick=()=>{b.textContent="Count: "+(++n)};fetch("/api/v1/bootstrap").then(()=>net.textContent="reached",()=>net.textContent="blocked");</script>`;
      add(c, "assistant", `Here you go.\n\n\`\`\`app\n${app}\n\`\`\``, { harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE });
      void emit(c);
      return;
    }
    // "gfm:" replies with GitHub-flavored markdown.
    if (prompt.startsWith("gfm:")) {
      add(c, "assistant", "| Option | Cost |\n|---|--:|\n| OpenAI tts-1 | $0.015 |\n| Cartesia | $0.011 |\n\n- [x] Shipped\n- [ ] Pending\n\n~~old~~\n\n> Quoted", { harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE });
      void emit(c);
      return;
    }
    // "recommend:" replies with a video, a site and a page with no preview.
    if (prompt.startsWith("recommend:")) {
      const fence = (u: string, why: string) => `\`\`\`recommend\n${u}\n${why}\n\`\`\``;
      add(c, "assistant", `Two things:\n\n${fence("https://www.youtube.com/watch?v=dQw4w9WgXcQ", "Short and useful.")}\n\n${fence("https://trailatlas.example/routes", "Start with the river loop.")}\n\nAnd:\n\n${fence("https://notes.example/grinders", "Best comparison.")}`, { harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE });
      void emit(c);
      return;
    }
    // "svg:" replies with an SVG image; "badsvg:" with one the browser can't draw.
    if (prompt.startsWith("svg:") || prompt.startsWith("badsvg:")) {
      const svg = prompt.startsWith("svg:") ? `<svg viewBox="0 0 40 20" width="40" height="20"><script>document.title="ran"</script><rect width="40" height="20" fill="#36c"/></svg>` : "<svg><rect></svg>";
      add(c, "assistant", `Here you go.\n\n\`\`\`svg-image\n${svg}\n\`\`\``, { harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE });
      void emit(c);
      return;
    }
    add(c, "assistant", `Echo: ${prompt}`, { harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE, ...(citations ? { citations } : {}) });
    void emit(c);
  }
  function ref(c: Chat) {
    const a = AGENTS.find((x) => x.id === c.agent_id)!;
    const p = personas.find((x) => x.id === c.persona_id);
    return {
      ...c, agent_id: undefined, persona_id: undefined,
      agent: { id: a.id, name: a.name, handle: a.handle, color_index: a.color_index },
      ...(p ? { persona: { id: p.id, name: p.name, color: p.color, ...(p.avatar_url ? { avatar_url: p.avatar_url } : {}), capabilities: p.capabilities } } : {}),
    };
  }
  const updatePersona = (id: string, patch: Partial<MockPersona>) => {
    const p = personas.find((x) => x.id === id)!;
    Object.assign(p, patch, { revision: p.revision + 1 });
    return page
      .evaluate((pid) => {
        const data = JSON.stringify({ type: "persona.updated", ids: [pid] });
        for (const s of (window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> }).__sse) s.onmessage?.({ data });
      }, id)
      .catch(() => undefined);
  };

  // Your profile (Settings → Profile): shared by /bootstrap and /me so edits show in the sidebar.
  const me: Record<string, unknown> & { id: string; name: string } = {
    id: "00000000-0000-4000-8000-0000000000aa", name: "Liam", email: "liam@example.com", initials: "L", role_title: "Owner",
  };
  const profileRequests: string[] = [];
  const initialsOf = (name: string) => name.trim().split(/\s+/).map((w) => w[0]).slice(0, 2).join("").toUpperCase();

  async function handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const method = req.method();
    const body = () => (req.postDataJSON() ?? {}) as Record<string, unknown>;
    const json = (data: unknown, status = 200) =>
      route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
    const fail = (e: ReturnType<typeof err>) => json(e.body, e.status);

    if (path === "/bootstrap") {
      return json({
        workspace: { id: uuid(), name: "Acme", slug: "acme" },
        me,
        users: [], agents: AGENTS, epics: [], labels: [],
        projects: [{ id: uuid(), key: "WEB", name: "Web app", color: "#6e56cf", default_branch: "main", archived: false, revision: 1 }],
        counts: { inbox: 0, wiki_suggestions: 0, live_agents: 0 },
      });
    }
    if (path === "/me" && method === "GET") return json(me);
    if (path === "/me" && method === "PATCH") {
      const b = body() as { name?: string; role_title?: string };
      profileRequests.push(`PATCH ${JSON.stringify(b)}`);
      if (b.name !== undefined && !b.name.trim()) return fail(err(422, "invalid", "Name is required.", "name"));
      if (b.name !== undefined) Object.assign(me, { name: b.name.trim(), initials: initialsOf(b.name) });
      if (b.role_title !== undefined) me.role_title = b.role_title.trim();
      return json(me);
    }
    if (path === "/me/avatar" && method === "PUT") {
      const raw = (req.postDataBuffer() ?? Buffer.alloc(0)).toString("latin1");
      profileRequests.push("PUT avatar");
      if (!raw.includes("\x89PNG") && !raw.includes("\xff\xd8\xff") && !raw.includes("WEBP")) return fail(err(415, "unsupported_type", "PNG, JPEG or WebP only"));
      me.avatar_url = `/api/v1/users/${me.id}/avatar?v=${profileRequests.length}`;
      return json(me);
    }
    if (path === "/me/avatar" && method === "DELETE") {
      profileRequests.push("DELETE avatar");
      delete me.avatar_url;
      return json(me);
    }
    if (/^\/users\/[^/]+\/avatar$/.test(path) && method === "GET") {
      const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", "base64");
      return route.fulfill({ status: 200, contentType: "image/png", body: png });
    }
    if (path === "/persona-dock") return json({ personas: [] });
    if (path === "/runner") return json({ online: true, runners: [], slots: 1, busy: 0, live: 0, queued: 0 });
    // Sidebar settings dot (LAZYAGENTS-44): both harness keys set, so no dot.
    if (path === "/secrets") return json({ key_configured: true, known: [], secrets: ["harness:anthropic", "harness:openai"].map((name, i) => ({ id: `secret-${i}`, name, created_at: "2026-11-01T00:00:00Z", updated_at: "2026-11-01T00:00:00Z" })) });
    if (path === "/saved-views" && method === "GET") return json({ views: [] });
    if (path === "/knowledge/resolve") {
      // Only the plan page resolves, so the live activity line can link it.
      return json({ links: url.searchParams.getAll("ref").filter((r) => r === PLAN.path).map((r) => ({ ref: r, target: PLAN })) });
    }
    if (path === "/knowledge/suggest") {
      const q = (url.searchParams.get("q") ?? "").toLowerCase();
      return json({ targets: TARGETS.filter((t) => !q || `${t.title} ${t.path ?? ""} ${t.display_id ?? ""}`.toLowerCase().includes(q)) });
    }
    if (path === "/wiki/pages" && method === "GET") {
      return json({ pages: [], suggestions: 0 });
    }
    if (path === "/wiki/pages" && method === "POST") {
      const b = body() as { path: string; body_md: string; project_key?: string };
      if (wikiPages.some((p) => p.path === b.path)) return fail(err(409, "conflict", "A page already exists at that path.", "path"));
      wikiPages.push(b);
      return json({ id: uuid(), type: "note", path: b.path, title: b.path.split("/").pop(), body_md: b.body_md }, 201);
    }
    if (path === "/artifacts" && method === "POST") {
      const raw = req.postData() ?? "";
      artifactForms.push(raw);
      const title = /name="title"\r\n\r\n([^\r]*)/.exec(raw)?.[1] ?? "";
      return json({ id: "66666666-6666-4666-8666-666666666666", type: "report", title }, 201);
    }
    // The chat header's spend chip: nothing spent, so no chip (an unhandled 404 would be retried and stall mutations).
    if (path === "/costs") {
      return json({
        total: { key: "", label: "", runs: 0, tokens: 0, cost_micros: 0, unpriced_runs: 0 }, unpriced_runs: 0, estimated: { tickets: 0, points: 0, cost_micros: 0 },
        days: [], agents: [], projects: [], models: [], purposes: [], top_tickets: [],
      });
    }
    if (path === "/chat-models") return json(catalog(codexAvailable, opencodeAvailable));

    if (path === "/memories" && method === "GET") {
      memoryRequests.push(`GET ${path}${url.search}`);
      const q = (url.searchParams.get("q") ?? "").toLowerCase();
      const type = url.searchParams.get("type");
      const ofType = memories.filter((x) => !type || x.type === type);
      const hits = ofType.filter((x) => !q || `${x.title} ${x.summary} ${x.tags.join(" ")}`.toLowerCase().includes(q));
      return json({ memories: hits, total: ofType.length, has_more: false });
    }
    if (path === "/memories" && method === "DELETE") {
      memoryRequests.push(`DELETE ${path}${url.search}`);
      if (url.searchParams.get("confirm") !== "true") return fail(err(422, "invalid", "confirm=true is required", "confirm"));
      const deleted = memories.length;
      memories.length = 0;
      return json({ deleted });
    }
    if (path === "/memories/export" && method === "GET") {
      memoryRequests.push(`GET ${path}`);
      return json({ exported_at: now(), memories });
    }
    if (path === "/memories/import" && method === "POST") {
      memoryRequests.push(`POST ${path}`);
      const b = body();
      memoryImports.push(b);
      const entries = (b.memories ?? []) as Array<Partial<MockMemory>>;
      let created = 0;
      let updated = 0;
      for (const e of entries) {
        const hit = memories.find((x) => x.title === e.title);
        const stamp = new Date(Date.now() + 1000).toISOString();
        if (hit) {
          Object.assign(hit, e, { updated_at: stamp });
          updated++;
        } else {
          const t = now();
          memories.unshift({
            id: `88888888-8888-4888-8888-${String(memories.length + 1).padStart(12, "0")}`,
            title: e.title ?? "", summary: e.summary ?? "", body: e.body ?? "", tags: e.tags ?? [], type: e.type ?? "other",
            source: "import", created_at: t, updated_at: t,
          });
          created++;
        }
      }
      return json({ created, updated, failed: [] });
    }
    const mem = /^\/memories\/([^/]+)$/.exec(path);
    if (mem) {
      memoryRequests.push(`${method} ${path}`);
      const i = memories.findIndex((x) => x.id === mem[1]);
      if (i < 0) return fail(err(404, "not_found", "Memory not found"));
      if (method === "GET") return json(memories[i]);
      if (method === "DELETE") {
        memories.splice(i, 1);
        return route.fulfill({ status: 204 });
      }
      if (method === "PATCH") {
        const b = body();
        memoryPatches.push(b);
        Object.assign(memories[i], b, { updated_at: new Date(Date.now() + 1000).toISOString() });
        return json(memories[i]);
      }
    }

    if (path === "/chats" && method === "GET") {
      const archived = url.searchParams.get("archived") === "true";
      const pid = url.searchParams.get("persona_id");
      chatPersonaFilters.push(pid ?? "");
      const want = pid ?? defaultPersona()?.id ?? "none";
      const list = chats
        .filter((c) => c.project === url.searchParams.get("project") && c.archived === archived)
        .filter((c) => want === "all" || (want === "none" ? !c.persona_id : c.persona_id === want))
        .sort((a, b) => (b.last_message_at ?? b.created_at).localeCompare(a.last_message_at ?? a.created_at));
      return json({ chats: list.map(ref), has_more: false });
    }
    if (path === "/chats" && method === "POST") {
      const b = body();
      const agent = b.agent_id === undefined ? AGENTS[0] : AGENTS.find((a) => a.id === b.agent_id);
      if (!agent) return fail(err(422, "invalid", "unknown agent", "agent_id"));
      if (agent.kind === "sub") return fail(err(422, "invalid", "pick a main agent", "agent_id"));
      if (b.persona_id !== undefined && !personas.some((p) => p.id === b.persona_id)) return fail(err(422, "invalid", "unknown persona", "persona_id"));
      created.push(b);
      const c: Chat = {
        id: uuid(), project: String(b.project), title: String(b.title ?? ""), agent_id: agent.id,
        harness: (b.harness as Harness) ?? "claude-code", model: String(b.model ?? "opus-5.5"), effort: b.effort as Effort | undefined,
        state: "idle", archived: false, revision: 1, message_count: 0, created_at: now(), updated_at: now(),
        persona_id: (b.persona_id as string | undefined) ?? defaultPersona()?.id,
      };
      // A coding persona's chat works in the requested repo (default: its first) on its own branch.
      const owner = personas.find((p) => p.id === c.persona_id);
      const repo = (b.repo_url as string | undefined) ?? owner?.repos[0];
      if (b.repo_url !== undefined && !(owner?.capabilities.code && owner.repos.includes(String(b.repo_url)))) return fail(err(422, "invalid", "not one of the persona's repos", "repo_url"));
      if (owner?.capabilities.code && repo) Object.assign(c, { repo_url: repo, work_branch: `vesper/${owner.name.toLowerCase()}-${c.id.slice(0, 8)}` });
      chats.unshift(c);
      messages.set(c.id, []);
      return json(ref(c), 201);
    }

    const pp = /^\/chats\/([^/]+)\/persona-proposals(?:\/([^/]+))?$/.exec(path);
    if (pp && method === "GET") return json({ proposals: proposals.filter((x) => x.chat_id === pp[1]) });
    if (pp && pp[2] && method === "POST") {
      const p = proposals.find((x) => x.id === pp[2]);
      if (!p) return fail(err(404, "not_found", "Proposal not found"));
      if (p.state !== "pending") return fail(err(409, "conflict", `already ${p.state}`));
      const decision = String(body().decision);
      decisions.push({ id: p.id, decision });
      p.state = decision === "approve" ? "approved" : "declined";
      return json(p);
    }

    const m = /^\/chats\/([^/]+)(?:\/(messages|cancel))?$/.exec(path);
    if (m) {
      const c = chats.find((x) => x.id === m[1]);
      if (!c) return fail(err(404, "not_found", "Chat not found"));
      const sub = m[2];
      if (!sub && method === "GET") return json(ref(c));
      if (!sub && method === "DELETE") {
        chats.splice(chats.indexOf(c), 1);
        return route.fulfill({ status: 204 });
      }
      if (!sub && method === "PATCH") {
        const b = body();
        if (b.revision !== c.revision) return fail(err(409, "conflict", "This chat changed. Reload and try again."));
        if (c.state === "running") return fail(err(409, "conflict", "A turn is running."));
        if (typeof b.title === "string") c.title = b.title;
        if (typeof b.archived === "boolean") c.archived = b.archived;
        touch(c);
        return json(ref(c));
      }
      if (sub === "messages" && method === "GET") {
        const after = Number(url.searchParams.get("after") ?? 0);
        return json({ messages: messages.get(c.id)!.filter((x) => x.seq > after), has_more: false });
      }
      if (sub === "messages" && method === "POST") {
        const b = body();
        const text = String(b.body ?? "");
        if (c.archived) return fail(err(409, "conflict", "This chat is archived."));
        if (!text.trim()) return fail(err(422, "invalid", "Message is empty", "body"));
        const harness = (b.harness as Harness | undefined) ?? c.harness;
        const model = (b.model as string | undefined) ?? (harness === c.harness ? c.model : catalog(true, true).find((g) => g.harness === harness)!.models[0].id);
        const g = catalog(codexAvailable, opencodeAvailable).find((x) => x.harness === harness);
        if (!g || !g.models.some((x) => x.id === model) || !g.available) {
          return fail(err(422, "invalid", "Unknown or unavailable harness/model", "model"));
        }
        const offered: string[] = g.models.find((x) => x.id === model)!.efforts;
        if (b.effort === "default") c.effort = undefined;
        else if (b.effort) {
          if (!offered.includes(b.effort as string)) return fail(err(422, "invalid", "Effort not offered", "effort"));
          c.effort = b.effort as Effort;
        } else if (c.effort && !offered.includes(c.effort)) c.effort = undefined;
        if (harness !== c.harness || model !== c.model) {
          c.harness = harness;
          c.model = model;
          add(c, "system", `Switched to ${label(harness, model)}`);
        }
        if (!c.title) c.title = text.slice(0, 40);
        posted.push({ chat: c.id, body: b });
        const refs = (b.context_refs as Array<{ key?: string }> | undefined) ?? [];
        if (refs.some((r) => r.key === "OTHER-9")) return fail(err(422, "invalid", "OTHER-9 is not in this chat's project", "context_refs"));
        const interrupted = c.state === "running" ? prompts.get(c.id) : undefined;
        clearTimeout(timers.get(c.id));
        const user = add(c, "user", text, refs.length ? { context_refs: refs } : {});
        const prompt = interrupted === undefined ? text : `${text} (interrupting: ${interrupted})`;
        prompts.set(c.id, prompt);
        c.state = "running";
        c.error = undefined;
        c.current_run_id = uuid();
        touch(c);
        if (text.startsWith("slow:")) {
          const events = runEvents.get(c.current_run_id) ?? [];
          events.push({ seq: events.length + 1, type: "tool_call", at: now(), payload: { id: `call-${events.length + 1}`, tool: "mcp__lazyagents__read_page", input: { ref: PLAN.path } } });
          runEvents.set(c.current_run_id, events);
        }
        if (!text.startsWith("slow:")) timers.set(c.id, setTimeout(() => finishTurn(c, prompt), delay));
        void emit(c);
        return json({ chat: ref(c), message: user }, 202);
      }
      if (sub === "cancel" && method === "POST") {
        if (c.state !== "running") return fail(err(409, "conflict", "No turn is running."));
        clearTimeout(timers.get(c.id));
        c.state = "idle";
        add(c, "system", "Turn cancelled");
        touch(c);
        void emit(c);
        return json(ref(c));
      }
    }

    if (path === "/personas" && method === "GET") {
      return json({ personas, ...(personas.find((x) => x.is_default) ? { default_persona_id: personas.find((x) => x.is_default)!.id } : {}) });
    }
    const personaBody = (b: Record<string, unknown>, self?: MockPersona) => {
      const name = typeof b.name === "string" ? b.name.trim() : undefined;
      if (name !== undefined && personas.some((x) => x !== self && x.name.toLowerCase() === name.toLowerCase()))
        return err(422, "invalid", "You already have a persona with this name", "name");
      if (typeof b.color === "string" && !/^#[0-9a-f]{6}$/i.test(b.color)) return err(422, "invalid", "Use #rrggbb", "color");
      return null;
    };
    if (path === "/personas" && method === "POST") {
      const b = body();
      personaRequests.push(`POST ${path} ${JSON.stringify(b)}`);
      const bad = personaBody(b);
      if (bad) return fail(bad);
      if (b.is_default) for (const x of personas) x.is_default = false;
      const t = now();
      const p: MockPersona = {
        id: uuid(), name: String(b.name).trim(), tone: (b.tone as string) ?? "neutral", color: String(b.color ?? "#64748b").toLowerCase(),
        custom_instructions: (b.custom_instructions as string) ?? "", is_default: !!b.is_default,
        capabilities: (b.capabilities as MockPersona["capabilities"] | undefined) ?? { code: false, images: false }, repos: (b.repos as string[] | undefined) ?? [], chat_count: 0, revision: 1, created_at: t, updated_at: t,
      };
      personas.push(p);
      return json(p, 201);
    }
    const pm = /^\/personas\/([^/]+)(?:\/(default|avatar))?$/.exec(path);
    if (pm) {
      const p = personas.find((x) => x.id === pm[1]);
      if (!p) return fail(err(404, "not_found", "persona not found"));
      personaRequests.push(`${method} ${path}`);
      const bump = () => {
        p.revision++;
        p.updated_at = now();
      };
      if (!pm[2] && method === "PATCH") {
        const b = body();
        if (b.revision !== p.revision) return fail(err(409, "conflict", "persona changed"));
        const bad = personaBody(b, p);
        if (bad) return fail(bad);
        for (const k of ["name", "tone", "custom_instructions"] as const) if (typeof b[k] === "string") p[k] = (b[k] as string).trim() || p[k];
        if (typeof b.custom_instructions === "string") p.custom_instructions = b.custom_instructions;
        if (typeof b.color === "string") p.color = b.color.toLowerCase();
        if (b.capabilities) p.capabilities = b.capabilities as MockPersona["capabilities"];
        if (Array.isArray(b.repos)) p.repos = b.repos as string[];
        bump();
        return json(p);
      }
      if (!pm[2] && method === "DELETE") {
        const rev = url.searchParams.get("revision");
        if (rev && Number(rev) !== p.revision) return fail(err(409, "conflict", "persona changed"));
        personas.splice(personas.indexOf(p), 1);
        // ON DELETE SET NULL: its chats fall back to the built-in Assistant.
        for (const c of chats) if (c.persona_id === p.id) c.persona_id = undefined;
        if (p.is_default && personas.length) {
          const t = (x: MockPersona) => Date.parse(x.last_used_at ?? "") || 0;
          [...personas].sort((a, b) => t(b) - t(a) || Date.parse(b.created_at) - Date.parse(a.created_at))[0].is_default = true;
        }
        return route.fulfill({ status: 204 });
      }
      if (pm[2] === "default" && method === "POST") {
        for (const x of personas) if (x.is_default && x !== p) {
          x.is_default = false;
          x.revision++;
        }
        if (!p.is_default) {
          p.is_default = true;
          bump();
        }
        return json(p);
      }
      if (pm[2] === "avatar" && method === "PUT") {
        const raw = req.postDataBuffer() ?? Buffer.alloc(0);
        if (raw.length > 1_048_576 + 4096) return fail(err(413, "too_large", "avatar over 1 MB"));
        const text = raw.toString("latin1");
        if (!text.includes("\x89PNG") && !text.includes("\xff\xd8\xff") && !text.includes("WEBP")) return fail(err(415, "unsupported_type", "PNG, JPEG or WebP only"));
        bump();
        p.avatar_url = `/api/v1/personas/${p.id}/avatar?v=${p.revision}`;
        return json(p);
      }
      if (pm[2] === "avatar" && method === "DELETE") {
        if (p.avatar_url) bump();
        delete p.avatar_url;
        return json(p);
      }
      if (pm[2] === "avatar" && method === "GET") {
        // A 1×1 PNG.
        const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", "base64");
        return route.fulfill({ status: 200, contentType: "image/png", body: png });
      }
    }

    if (/^\/chat-messages\/[^/]+\/link-previews$/.test(path)) return json({ previews: [
      { url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", kind: "video", title: "Why small rooms sound better", description: "", site: "Room Acoustics Lab",  saved: false, video_id: "dQw4w9WgXcQ", image_url: "/api/v1/link-previews/thumb/image" },
      { url: "https://trailatlas.example/routes", kind: "site", title: "A field guide to quiet bike routes", description: "Routes", site: "", saved: false, image_url: "/api/v1/link-previews/site/image" },
      { url: "https://notes.example/grinders", kind: "site", title: "", description: "", site: "", saved: false },
    ] });
    if (path === "/library/links" && method === "POST") return json({ id: "11111111-aaaa-4aaa-8aaa-111111111111", kind: "link", title: "T", filename: "link", content_type: "image/png", size_bytes: 9, created_at: now() }, 201);
    if (/^\/link-previews\/[^/]+\/image$/.test(path)) {
      const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", "base64");
      return route.fulfill({ status: 200, contentType: "image/png", body: png });
    }

    const run = /^\/runs\/([^/]+)$/.exec(path);
    if (run) {
      return json({ run: {}, turns: [], approvals: [], events: runEvents.get(run[1]) ?? [] });
    }
    unhandled.push(`${method} ${path}`);
    return fail(err(404, "not_found", `mock: no handler for ${method} ${path}`));
  }

  return (async () => {
    // Replace EventSource with a stub that reports "open" so the shell shows no
    // offline banner; `emit` pushes a hint like the real /events stream does.
    await page.addInitScript(() => {
      const w = window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> };
      w.__sse = [];
      class FakeEventSource {
        onopen?: () => void;
        onerror?: () => void;
        onmessage?: (e: { data: string }) => void;
        constructor() {
          w.__sse.push(this);
          setTimeout(() => this.onopen?.(), 0);
        }
        close() {
          w.__sse = w.__sse.filter((x) => x !== this);
        }
      }
      (window as unknown as { EventSource: unknown }).EventSource = FakeEventSource;
    });
    await page.route("**/api/v1/**", handle);
    return { proposals, decisions, chats, unhandled, posted, created, wikiPages, artifactForms, memories, memoryRequests, memoryPatches, memoryImports, personas, personaRequests, chatPersonaFilters, updatePersona, profileRequests };
  })();
}
