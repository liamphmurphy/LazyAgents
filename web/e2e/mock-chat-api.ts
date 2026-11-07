import type { Page, Route } from "@playwright/test";

/**
 * In-memory stand-in for the chat REST contract (api/openapi.yaml, tag `chats`)
 * plus the fake harness: an assistant reply that echoes the prompt, tagged with
 * the selected harness/model. Swap `installChatApi` for the real stack once
 * LAZYAGENTS-11 lands; the spec only talks to the UI.
 *
 * A message starting with "slow:" keeps the turn running until it is cancelled;
 * its run reports one tool call (read_page Eng/Plan) for the live activity line.
 * Replies carry token usage and cost and a run_id whose events (GET /runs/{id}) hold the turn's tool calls.
 *
 * Assistant memory (LAZYAGENTS-71): an in-memory store behind /memories. A message
 * "remember: <title>" makes the turn call save_memory (creating the entry, sourced
 * from the chat) and "forget: <title>" calls forget_memory (deleting it); both put a
 * successful tool_result into the turn's run events, between the user message and the reply.
 */

type Harness = "claude-code" | "codex" | "opencode";
type State = "idle" | "running" | "failed";
type Chat = {
  id: string; project: string; title: string; agent_id: string; harness: Harness; model: string; state: State;
  error?: string; current_run_id?: string; archived: boolean; revision: number; message_count: number;
  created_at: string; updated_at: string; last_message_at?: string;
};
type Message = {
  id: string; seq: number; role: "user" | "assistant" | "system"; body_md: string;
  harness?: Harness; model?: string; created_at: string;
  context_refs?: unknown[]; citations?: unknown[]; run_id?: string;
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
  is_default: boolean; chat_count: number; last_used_at?: string; revision: number; created_at: string; updated_at: string;
};
export type MockOptions = { codexAvailable?: boolean; opencodeAvailable?: boolean; replyDelayMs?: number; personas?: MockPersona[] };

/** A persona fixture for the persona specs. */
export function persona(i: number, over: Partial<MockPersona> = {}): MockPersona {
  const t = `2026-11-0${Math.min(i, 9)}T09:00:00.000Z`;
  return {
    id: `77777777-7777-4777-8777-${String(i).padStart(12, "0")}`,
    name: `Persona ${i}`, tone: "neutral", color: "#8b8fff", custom_instructions: "", is_default: false,
    chat_count: 0, revision: 1, created_at: t, updated_at: t, ...over,
  };
}

export const catalog = (codexAvailable: boolean, opencodeAvailable = true) => [
  {
    harness: "claude-code", label: "Claude Code", available: true,
    models: [
      { id: "opus-5.5", label: "Opus 5.5", default: true },
      { id: "sonnet-5.5", label: "Sonnet 5.5", default: false },
    ],
  },
  {
    harness: "codex", label: "Codex", available: codexAvailable,
    ...(codexAvailable ? {} : { unavailable_reason: "No harness:openai secret" }),
    models: [{ id: "gpt-5-codex", label: "gpt-5-codex", default: true }],
  },
  {
    harness: "opencode", label: "OpenCode", available: opencodeAvailable,
    ...(opencodeAvailable ? {} : { unavailable_reason: "OpenCode is not installed" }),
    models: [
      { id: "anthropic/claude-sonnet-4-5", label: "anthropic/claude-sonnet-4-5", default: true },
      { id: "openai/gpt-5", label: "openai/gpt-5", default: false },
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
  const personaRequests: string[] = [];
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

  function finishTurn(c: Chat, prompt: string) {
    c.state = "idle";
    memoryEvents(c, prompt);
    // "cite:" prompts get citations (one duplicate, one versioned section, one ticket).
    const citations = prompt.startsWith("cite:")
      ? [
          { target: REPORT, artifact_id: REPORT.id, title: REPORT.title, version_id: "55555555-5555-4555-8555-555555555555", version: "v2", section: "risks" },
          { target: REPORT, artifact_id: REPORT.id, title: REPORT.title, version_id: "55555555-5555-4555-8555-555555555555", version: "v2", section: "risks" },
          { target: PLAN, artifact_id: PLAN.id, title: PLAN.title, path: PLAN.path },
          { target: TICKET, title: TICKET.title },
        ]
      : undefined;
    add(c, "assistant", `Echo: ${prompt}`, { harness: c.harness, model: c.model, run_id: c.current_run_id, ...USAGE, ...(citations ? { citations } : {}) });
    void emit(c);
  }
  function ref(c: Chat) {
    const a = AGENTS.find((x) => x.id === c.agent_id)!;
    return { ...c, agent_id: undefined, agent: { id: a.id, name: a.name, handle: a.handle, color_index: a.color_index } };
  }

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
        me: { id: uuid(), name: "Liam", email: "liam@example.com", initials: "L", role_title: "Owner" },
        users: [], agents: AGENTS, epics: [], labels: [],
        projects: [{ id: uuid(), key: "WEB", name: "Web app", color: "#6e56cf", default_branch: "main", archived: false, revision: 1 }],
        counts: { inbox: 0, wiki_suggestions: 0, live_agents: 0 },
      });
    }
    if (path === "/runner") return json({ online: true, runners: [], slots: 1, busy: 0, queued: 0 });
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
      const list = chats
        .filter((c) => c.project === url.searchParams.get("project") && c.archived === archived)
        .sort((a, b) => (b.last_message_at ?? b.created_at).localeCompare(a.last_message_at ?? a.created_at));
      return json({ chats: list.map(ref), has_more: false });
    }
    if (path === "/chats" && method === "POST") {
      const b = body();
      const agent = b.agent_id === undefined ? AGENTS[0] : AGENTS.find((a) => a.id === b.agent_id);
      if (!agent) return fail(err(422, "invalid", "unknown agent", "agent_id"));
      if (agent.kind === "sub") return fail(err(422, "invalid", "pick a main agent", "agent_id"));
      created.push(b);
      const c: Chat = {
        id: uuid(), project: String(b.project), title: String(b.title ?? ""), agent_id: agent.id,
        harness: (b.harness as Harness) ?? "claude-code", model: String(b.model ?? "opus-5.5"),
        state: "idle", archived: false, revision: 1, message_count: 0, created_at: now(), updated_at: now(),
      };
      chats.unshift(c);
      messages.set(c.id, []);
      return json(ref(c), 201);
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
        if (c.state === "running") return fail(err(409, "conflict", "A turn is already running."));
        if (!text.trim()) return fail(err(422, "invalid", "Message is empty", "body"));
        const harness = (b.harness as Harness | undefined) ?? c.harness;
        const model = (b.model as string | undefined) ?? (harness === c.harness ? c.model : catalog(true, true).find((g) => g.harness === harness)!.models[0].id);
        const g = catalog(codexAvailable, opencodeAvailable).find((x) => x.harness === harness);
        if (!g || !g.models.some((x) => x.id === model) || !g.available) {
          return fail(err(422, "invalid", "Unknown or unavailable harness/model", "model"));
        }
        if (harness !== c.harness || model !== c.model) {
          c.harness = harness;
          c.model = model;
          add(c, "system", `Switched to ${label(harness, model)}`);
        }
        if (!c.title) c.title = text.slice(0, 40);
        posted.push({ chat: c.id, body: b });
        const refs = (b.context_refs as Array<{ key?: string }> | undefined) ?? [];
        if (refs.some((r) => r.key === "OTHER-9")) return fail(err(422, "invalid", "OTHER-9 is not in this chat's project", "context_refs"));
        const user = add(c, "user", text, refs.length ? { context_refs: refs } : {});
        c.state = "running";
        c.error = undefined;
        c.current_run_id = uuid();
        touch(c);
        if (text.startsWith("slow:")) {
          const events = runEvents.get(c.current_run_id) ?? [];
          events.push({ seq: events.length + 1, type: "tool_call", at: now(), payload: { id: `call-${events.length + 1}`, tool: "mcp__lazyagents__read_page", input: { ref: PLAN.path } } });
          runEvents.set(c.current_run_id, events);
        }
        if (!text.startsWith("slow:")) timers.set(c.id, setTimeout(() => finishTurn(c, text), delay));
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
        custom_instructions: (b.custom_instructions as string) ?? "", is_default: !!b.is_default, chat_count: 0, revision: 1, created_at: t, updated_at: t,
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
        bump();
        return json(p);
      }
      if (!pm[2] && method === "DELETE") {
        const rev = url.searchParams.get("revision");
        if (rev && Number(rev) !== p.revision) return fail(err(409, "conflict", "persona changed"));
        personas.splice(personas.indexOf(p), 1);
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
    return { unhandled, posted, created, wikiPages, artifactForms, memories, memoryRequests, memoryPatches, memoryImports, personas, personaRequests };
  })();
}
