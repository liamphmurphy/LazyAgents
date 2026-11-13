// A persona's presence on a design (ADR-designs §3.7), read from the design
// chat's running turn: no new transport. Its design_* tool calls name the
// canvas and the nodes it reads or patches; the newest call with nodes says
// where its cursor is.
import { bareTool, resultJSON, type ToolCall } from "../chat-ui.ts";

export type Presence = {
  /** A design tool call is in flight (rather than the persona thinking between calls). */
  busy: boolean;
  /** The design tool it called last. */
  tool: string;
  /** The nodes it is working on, in the order it touched them; empty when it named none. */
  nodes: string[];
  /** The nodes that call's input names: where the cursor waits while nodes it just added haven't reached this tab. */
  around: string[];
};

const NODE_KEYS = ["id", "parent", "after", "from", "to", "node", "component"];
const isNode = (s: unknown): s is string => typeof s === "string" && /^n_[a-z0-9_]{1,40}$/.test(s);

/** The node ids a design tool call's input names. */
export function nodesOf(input: unknown): string[] {
  const o = input && typeof input === "object" ? (input as Record<string, unknown>) : {};
  const out = new Set<string>();
  const add = (v: unknown) => isNode(v) && out.add(v);
  add(o.node_id);
  if (Array.isArray(o.node_ids)) o.node_ids.forEach(add);
  if (Array.isArray(o.ops)) {
    for (const op of o.ops) {
      if (!op || typeof op !== "object") continue;
      for (const k of NODE_KEYS) add((op as Record<string, unknown>)[k]);
      const node = (op as Record<string, unknown>).node;
      if (node && typeof node === "object") add((node as Record<string, unknown>).id);
    }
  }
  return [...out];
}

/**
 * The nodes a call touched. A finished design_patch lists them in its result, by the ids the
 * server minted for nodes the persona left unnamed, which its input can't know.
 */
function touched(c: ToolCall): string[] {
  const listed = c.done ? resultJSON(c.output)?.nodes : undefined;
  const nodes = Array.isArray(listed) ? listed.filter(isNode) : [];
  return nodes.length ? nodes : nodesOf(c.input);
}

/** The persona's presence on a canvas while its turn runs; undefined when it isn't working on it. */
export function presenceOf(calls: ToolCall[], canvasId: string, running: boolean): Presence | undefined {
  if (!running) return undefined;
  const mine = calls.filter((c) => bareTool(c.tool).startsWith("design_") && (c.input as { canvas_id?: unknown } | undefined)?.canvas_id === canvasId);
  const last = mine.at(-1);
  if (!last) return undefined;
  // design_list / design_create don't name a node; the last call that did does.
  const named = [...mine].reverse().find((c) => touched(c).length);
  return { busy: !last.done, tool: bareTool(last.tool), nodes: named ? touched(named) : [], around: named ? nodesOf(named.input) : [] };
}
