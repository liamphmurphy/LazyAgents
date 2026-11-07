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

/** "a, B ,a,,c" → ["a", "b", "c"]: lowercase, trimmed, deduped, at most 12. */
export function parseTags(text: string): string[] {
  const out: string[] = [];
  for (const t of text.split(",")) {
    const v = t.trim().toLowerCase();
    if (v && !out.includes(v)) out.push(v);
  }
  return out.slice(0, 12);
}
