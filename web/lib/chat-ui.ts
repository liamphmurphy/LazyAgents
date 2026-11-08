// Pure logic for the chat screen (LAZYAGENTS-57): the live activity line, when a model switch asks first, and reply cost.
// Kept free of React so `npm run test:unit` covers it.

export type RunEventLike = { seq: number; type: string; at: string; payload: Record<string, unknown> };
export type ToolCall = { key: number; tool: string; input: unknown; done: boolean };

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
      out[open.get(str(p.id))!].done = true;
    }
  }
  return out;
}

/** "mcp__lazyagents__read_page" (Claude) and "read_page" (Codex) are the same tool. */
export function bareTool(tool: string): string {
  const mcp = /^mcp__(.+?)__(.+)$/.exec(tool);
  if (mcp) return mcp[1] === "lazyagents" ? mcp[2] : `${mcp[1]}.${mcp[2]}`;
  return tool.startsWith("lazyagents.") ? tool.slice("lazyagents.".length) : tool;
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
    default:
      return { verb: "Using", text: bareTool(call.tool) || "a tool" };
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
