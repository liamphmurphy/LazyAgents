import type { Page, Route } from "@playwright/test";

/**
 * In-memory stand-in for the chat REST contract (api/openapi.yaml, tag `chats`)
 * plus the fake harness: an assistant reply that echoes the prompt, tagged with
 * the selected harness/model. Swap `installChatApi` for the real stack once
 * LAZYAGENTS-11 lands; the spec only talks to the UI.
 *
 * A message starting with "slow:" keeps the turn running until it is cancelled.
 */

type Harness = "claude-code" | "codex";
type State = "idle" | "running" | "failed";
type Chat = {
  id: string; project: string; title: string; harness: Harness; model: string; state: State;
  error?: string; current_run_id?: string; archived: boolean; revision: number; message_count: number;
  created_at: string; updated_at: string; last_message_at?: string;
};
type Message = {
  id: string; seq: number; role: "user" | "assistant" | "system"; body_md: string;
  harness?: Harness; model?: string; created_at: string;
};

export type MockOptions = { codexAvailable?: boolean; replyDelayMs?: number };

const catalog = (codexAvailable: boolean) => [
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
];

const err = (status: number, code: string, message: string, field?: string) => ({
  status,
  body: { code, message, ...(field ? { field } : {}) },
});

export function installChatApi(page: Page, opts: MockOptions = {}) {
  const codexAvailable = opts.codexAvailable ?? true;
  const delay = opts.replyDelayMs ?? 300;
  const chats: Chat[] = [];
  const messages = new Map<string, Message[]>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const unhandled: string[] = [];
  let n = 0;
  const uuid = () => `00000000-0000-4000-8000-${String(++n).padStart(12, "0")}`;
  const now = () => new Date().toISOString();
  const label = (h: Harness, m: string) => {
    const g = catalog(true).find((x) => x.harness === h)!;
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

  function finishTurn(c: Chat, prompt: string) {
    c.state = "idle";
    add(c, "assistant", `Echo: ${prompt}`, { harness: c.harness, model: c.model });
    void emit(c);
  }
  function ref(c: Chat) {
    return {
      ...c,
      agent: { id: uuid(), name: "Atlas", handle: "atlas", color_index: 0 },
    };
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
        users: [], agents: [], epics: [], labels: [],
        projects: [{ id: uuid(), key: "WEB", name: "Web app", color: "#6e56cf", default_branch: "main", archived: false, revision: 1 }],
        counts: { inbox: 0, wiki_suggestions: 0, live_agents: 0 },
      });
    }
    if (path === "/knowledge/resolve") return json({ links: [] });
    if (path === "/chat-models") return json(catalog(codexAvailable));

    if (path === "/chats" && method === "GET") {
      const archived = url.searchParams.get("archived") === "true";
      const list = chats
        .filter((c) => c.project === url.searchParams.get("project") && c.archived === archived)
        .sort((a, b) => (b.last_message_at ?? b.created_at).localeCompare(a.last_message_at ?? a.created_at));
      return json({ chats: list.map(ref), has_more: false });
    }
    if (path === "/chats" && method === "POST") {
      const b = body();
      const c: Chat = {
        id: uuid(), project: String(b.project), title: String(b.title ?? "New chat"),
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
        const model = (b.model as string | undefined) ?? (harness === c.harness ? c.model : catalog(true).find((g) => g.harness === harness)!.models[0].id);
        const g = catalog(codexAvailable).find((x) => x.harness === harness);
        if (!g || !g.models.some((x) => x.id === model) || !g.available) {
          return fail(err(422, "invalid", "Unknown or unavailable harness/model", "model"));
        }
        if (harness !== c.harness || model !== c.model) {
          c.harness = harness;
          c.model = model;
          add(c, "system", `Switched to ${label(harness, model)}`);
        }
        if (c.title === "New chat") c.title = text.slice(0, 40);
        const user = add(c, "user", text);
        c.state = "running";
        c.error = undefined;
        c.current_run_id = c.current_run_id ?? uuid();
        touch(c);
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

    const run = /^\/runs\/([^/]+)$/.exec(path);
    if (run) {
      return json({ run: {}, turns: [], approvals: [], events: [] });
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
    return { unhandled };
  })();
}
