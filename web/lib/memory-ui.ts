// Pure logic for the Personal Assistant's memory surfaces (LAZYAGENTS-71): inline
// "saved to memory" notices read from a turn's tool results, and the memory export.
// Kept free of React so `npm run test:unit` covers it.

import { bareTool, type RunEventLike } from "./chat-ui.ts";

export const MEMORY_TYPES = ["preference", "fact", "project", "person", "work", "life", "other"] as const;

export const typeLabel = (t: string) => (t ? t[0].toUpperCase() + t.slice(1) : "Other");

/** What a memory tool did in a turn: one notice under the reply. */
export type MemoryNotice = {
  key: number;
  kind: "saved" | "updated" | "forgotten";
  id: string;
  title: string;
  /** For a near-duplicate save: the entry as it was, so Undo can put it back. */
  previous?: { title: string; summary: string; type: string };
};

const MEMORY_TOOLS = new Set(["save_memory", "update_memory", "forget_memory"]);

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => !!v && typeof v === "object" && !Array.isArray(v);
const s = (v: unknown) => (typeof v === "string" ? v : "");

/** A tool result's JSON. Harnesses hand it over as text, sometimes still wrapped in MCP content blocks. */
export function toolOutput(output: unknown): Obj | undefined {
  let v: unknown = output;
  for (let i = 0; i < 3; i++) {
    if (typeof v === "string") {
      try {
        v = JSON.parse(v);
      } catch {
        return undefined;
      }
      continue;
    }
    if (isObj(v) && isObj(v.structuredContent)) return v.structuredContent;
    const blocks = isObj(v) && Array.isArray(v.content) ? v.content : Array.isArray(v) ? v : undefined;
    if (blocks) {
      const text = blocks.find((b) => isObj(b) && typeof b.text === "string") as Obj | undefined;
      v = text?.text;
      continue;
    }
    return isObj(v) ? v : undefined;
  }
  return isObj(v) ? v : undefined;
}

/**
 * Memory notices for one turn: successful save/update/forget_memory results with
 * since < at <= until (either bound optional). The tool name comes from the result,
 * or from its call when a harness leaves it off.
 */
export function memoryNotices(events: RunEventLike[] | undefined, since?: string, until?: string): MemoryNotice[] {
  const lo = since ? Date.parse(since) : -Infinity;
  const hi = until ? Date.parse(until) : Infinity;
  const tools = new Map<string, string>();
  const out: MemoryNotice[] = [];
  for (const e of events ?? []) {
    const p = e.payload;
    if (e.type === "tool_call" && p.id) tools.set(String(p.id), s(p.tool));
    if (e.type !== "tool_result" || p.is_error) continue;
    const at = Date.parse(e.at);
    if (!(at > lo && at <= hi)) continue;
    const tool = bareTool(s(p.tool) || tools.get(String(p.id ?? "")) || "");
    if (!MEMORY_TOOLS.has(tool)) continue;
    const o = toolOutput(p.output);
    if (!o) continue;
    if (tool === "forget_memory") {
      const id = s(o.forgotten);
      if (id) out.push({ key: e.seq, kind: "forgotten", id, title: s(o.title) });
    } else if (tool === "save_memory") {
      const id = s(o.id);
      if (!id) continue;
      const prev = isObj(o.previous) ? o.previous : undefined;
      out.push({
        key: e.seq,
        kind: o.action === "updated" ? "updated" : "saved",
        id,
        title: s(o.title),
        ...(prev ? { previous: { title: s(prev.title), summary: s(prev.summary), type: s(prev.type) } } : {}),
      });
    } else {
      const m = isObj(o.memory) ? o.memory : undefined;
      if (m && s(m.id)) out.push({ key: e.seq, kind: "updated", id: s(m.id), title: s(m.title) });
    }
  }
  return out;
}

type MemoryLike = {
  id: string;
  title: string;
  summary: string;
  body: string;
  tags: string[];
  type: string;
  source: string;
  superseded_by?: string;
  created_at: string;
  updated_at: string;
};

/** The whole memory as Markdown: one section per entry, retired ones marked. */
export function memoriesMarkdown(memories: MemoryLike[], exportedAt: string): string {
  const lines = [`# Assistant memory`, ``, `Exported ${exportedAt}. ${memories.length} entries.`, ``];
  for (const m of memories) {
    lines.push(`## ${m.title}${m.superseded_by ? " (retired)" : ""}`, ``);
    if (m.summary) lines.push(m.summary, ``);
    if (m.body && m.body !== m.summary) lines.push(m.body, ``);
    lines.push(`- Type: ${m.type}`);
    if (m.tags.length) lines.push(`- Tags: ${m.tags.join(", ")}`);
    lines.push(`- Source: ${m.source || "unknown"}`, `- Created: ${m.created_at}`, `- Updated: ${m.updated_at}`);
    if (m.superseded_by) lines.push(`- Superseded by: ${m.superseded_by}`);
    lines.push(`- Id: ${m.id}`, ``);
  }
  return lines.join("\n");
}

/** "a, B ,a,,c" → ["a", "b", "c"]: lowercase, trimmed, '#' stripped, deduped, none over 40 chars, at most 12. */
export function parseTags(text: string): string[] {
  const out: string[] = [];
  for (const t of text.split(",")) {
    const v = t.trim().replace(/^#+/, "").trim().toLowerCase();
    if (v && v.length <= 40 && !out.includes(v)) out.push(v);
  }
  return out.slice(0, 12);
}

export const IMPORT_PROMPT = `I'm moving to a new personal assistant called Vesper and want to bring over what you know about me. List everything you remember about me from our conversations and your saved memory: my preferences, facts about me, the people in my life, my work and projects, and anything else a personal assistant should know. Leave out passwords, API keys, tokens and other secrets.

Reply with only one JSON code block in exactly this shape, one object per distinct thing:

\`\`\`json
{"memories": [
  {"title": "Prefers metric units", "summary": "Uses kilometres and Celsius, not miles or Fahrenheit.", "body": "", "type": "preference", "tags": ["units"]}
]}
\`\`\`

Rules:
- title: a short name, under 80 characters.
- summary: one sentence saying what to remember.
- body: extra detail only when the summary isn't enough, otherwise "".
- type: one of preference, fact, project, person, work, life, other.
- tags: 1 to 4 lowercase keywords.
- Keep unrelated things in separate entries. Don't invent anything.`;

export const IMPORT_MAX = 200;

export type ImportEntry = { title: string; summary: string; body: string; tags: string[]; type: string };
export type ImportParse = { entries: ImportEntry[]; skipped: number; error?: string };

function tryJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

function findJson(text: string): unknown {
  const whole = tryJson(text);
  if (whole !== undefined) return whole;
  const fence = /```[a-zA-Z]*[ \t]*\r?\n?([\s\S]*?)```/.exec(text);
  if (fence) {
    const v = tryJson(fence[1].trim());
    if (v !== undefined) return v;
  }
  const a = text.search(/[{[]/);
  if (a >= 0) {
    const end = Math.max(text.lastIndexOf("}"), text.lastIndexOf("]"));
    if (end > a) return tryJson(text.slice(a, end + 1));
  }
  return undefined;
}

const oneLine = (v: string) => v.replace(/\s+/g, " ").trim();

/** Lenient read of another assistant's reply (or our own JSON export) into memory entries. */
export function parseMemoryImport(text: string): ImportParse {
  const trimmed = text.trim();
  if (!trimmed) return { entries: [], skipped: 0 };
  const json = findJson(trimmed);
  if (json === undefined || json === null || typeof json !== "object")
    return { entries: [], skipped: 0, error: "Couldn't find JSON in that text. Paste the whole reply, including the code block." };
  const list = Array.isArray(json) ? json : isObj(json) && Array.isArray(json.memories) ? json.memories : undefined;
  if (!list) return { entries: [], skipped: 0, error: "That JSON has no memories list." };
  if (list.length > IMPORT_MAX)
    return { entries: [], skipped: 0, error: `That's ${list.length} memories. Import at most ${IMPORT_MAX} at a time.` };
  const entries: ImportEntry[] = [];
  let skipped = 0;
  for (const item of list) {
    if (!isObj(item) || s(item.superseded_by)) {
      skipped++;
      continue;
    }
    const summary = oneLine(s(item.summary)).slice(0, 300);
    const title = (s(item.title).trim() || summary).slice(0, 200).trim();
    if (!title) {
      skipped++;
      continue;
    }
    const tags = Array.isArray(item.tags)
      ? parseTags(item.tags.filter((t): t is string => typeof t === "string").join(","))
      : parseTags(s(item.tags));
    const type = s(item.type).trim().toLowerCase();
    entries.push({
      title,
      summary,
      body: s(item.body).trim().slice(0, 20000),
      tags,
      type: (MEMORY_TYPES as readonly string[]).includes(type) ? type : "other",
    });
  }
  return { entries, skipped };
}
