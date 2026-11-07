// Pure logic for the assistant's web search surface (LAZYAGENTS-72): the queries,
// sources and opened pages of one turn, read from run events. Kept free of React
// so `npm run test:unit` covers it.

import { bareTool, type RunEventLike } from "./chat-ui.ts";

export type WebSource = { title: string; url: string; host: string; cited?: boolean };
export type WebSearches = { queries: string[]; sources: WebSource[]; fetched: string[]; running: boolean };

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => !!v && typeof v === "object" && !Array.isArray(v);
const s = (v: unknown) => (typeof v === "string" ? v.trim() : "");

/** "https://www.example.com/a" → "example.com"; anything unparsable comes back as is. */
export function hostOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

/** A tool result as text: a string, or MCP content blocks carrying text. */
function outputText(output: unknown): string {
  if (typeof output === "string") return output;
  const blocks = Array.isArray(output) ? output : isObj(output) && Array.isArray(output.content) ? output.content : [];
  return blocks.map((b) => (isObj(b) && typeof b.text === "string" ? b.text : "")).join("\n");
}

/** The links of a Claude WebSearch result: a `Links: [{"title":…,"url":…}]` line. */
export function searchLinks(output: unknown): { title: string; url: string }[] {
  const m = /Links:\s*(\[.*\])/.exec(outputText(output));
  if (!m) return [];
  try {
    const v: unknown = JSON.parse(m[1]);
    if (!Array.isArray(v)) return [];
    return v.flatMap((l) => (isObj(l) && s(l.url) ? [{ title: s(l.title), url: s(l.url) }] : []));
  } catch {
    return [];
  }
}

/**
 * Web searches and page reads for one turn: calls with since < at <= until (either
 * bound optional), like memoryNotices. Sources are deduped by url; opened pages are
 * sources too. `running` is a WebSearch call whose result hasn't arrived (Codex's
 * web_search has no result event, so it never runs).
 */
export function webSearches(events: RunEventLike[] | undefined, since?: string, until?: string): WebSearches {
  const lo = since ? Date.parse(since) : -Infinity;
  const hi = until ? Date.parse(until) : Infinity;
  const answered = new Set<string>();
  for (const e of events ?? []) if (e.type === "tool_result" && e.payload.id != null) answered.add(String(e.payload.id));
  const out: WebSearches = { queries: [], sources: [], fetched: [], running: false };
  const seen = new Set<string>();
  const add = (url: string, title: string) => {
    if (!url || seen.has(url)) return;
    seen.add(url);
    const host = hostOf(url);
    out.sources.push({ title: title || host, url, host });
  };
  const searches = new Set<string>();
  for (const e of events ?? []) {
    const p = e.payload;
    const at = Date.parse(e.at);
    if (e.type === "tool_call") {
      if (!(at > lo && at <= hi)) continue;
      const tool = bareTool(s(p.tool));
      const input = isObj(p.input) ? p.input : {};
      if (tool === "WebSearch" || tool === "web_search") {
        const q = s(input.query);
        if (q && !out.queries.includes(q)) out.queries.push(q);
        if (tool === "WebSearch") {
          if (p.id != null) searches.add(String(p.id));
          if (p.id == null || !answered.has(String(p.id))) out.running = true;
        }
      } else if (tool === "WebFetch") {
        const url = s(input.url);
        if (url && !out.fetched.includes(url)) out.fetched.push(url);
        add(url, "");
      }
    } else if (e.type === "tool_result" && !p.is_error && p.id != null && searches.has(String(p.id))) {
      for (const l of searchLinks(p.output)) add(l.url, l.title);
    }
  }
  return out;
}
