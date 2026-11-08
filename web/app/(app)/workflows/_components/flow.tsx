"use client";

import { Handle, Position, type Edge, type Node, type NodeProps } from "@xyflow/react";
import { memo, type CSSProperties } from "react";
import { cx } from "@/lib/cx";
import { NODE_LABEL, isOutput, nodeSummary, type Graph, type GraphNode, type NodeDiff } from "@/lib/workflows";

export type StepStatus = "running" | "waiting" | "done" | "failed" | "stale" | undefined;
export type FlowData = { node: GraphNode; diff?: NodeDiff; status?: StepStatus };
export type FlowNode = Node<FlowData, "step">;

export const FLOW_STYLE = {
  "--xy-background-color": "transparent",
  "--xy-background-pattern-dots-color": "var(--border-strong)",
  "--xy-edge-stroke": "var(--state-backlog)",
  "--xy-edge-stroke-selected": "var(--accent)",
  "--xy-connectionline-stroke": "var(--accent)",
  "--xy-controls-button-background-color": "var(--surface-raised)",
  "--xy-controls-button-background-color-hover": "var(--hover)",
  "--xy-controls-button-color": "var(--text)",
  "--xy-controls-button-border-color": "var(--border)",
  "--xy-attribution-background-color": "transparent",
} as CSSProperties;

const STATUS: Record<NonNullable<StepStatus>, string> = {
  running: "border-state-progress",
  waiting: "border-state-human bg-pill-human-bg",
  done: "border-state-done",
  failed: "border-state-blocked",
  stale: "border-border-strong",
};

const DIFF: Record<NonNullable<NodeDiff>, string> = {
  added: "border-dashed border-accent bg-accent-soft",
  changed: "border-accent",
  removed: "border-dashed border-state-blocked opacity-60",
};

const DIFF_WORD: Record<NonNullable<NodeDiff>, string> = { added: "+ ", changed: "~ ", removed: "− " };

function StepNode({ data, selected }: NodeProps<FlowNode>) {
  const { node: n, diff, status } = data;
  return (
    <div
      data-testid={`node-${n.id}`}
      className={cx("w-[168px] rounded-[10px] border bg-surface px-2.5 py-2", selected ? "border-2 border-accent" : status ? STATUS[status] : diff ? DIFF[diff] : "border-border-strong")}
    >
      {n.type !== "trigger" && <Handle type="target" position={Position.Left} />}
      <p className={cx("text-[9.5px] font-semibold uppercase tracking-wide", n.type === "gate" ? "text-state-human" : "text-fg-tertiary")}>
        {diff && DIFF_WORD[diff]}{NODE_LABEL[n.type]}
      </p>
      <p className="truncate text-[13px] font-semibold text-fg">{n.title || "Untitled"}</p>
      <p className="truncate text-[10.5px] text-fg-tertiary">{status === "waiting" ? "Waiting for you" : nodeSummary(n)}</p>
      {!isOutput(n.type) && <Handle type="source" position={Position.Right} />}
    </div>
  );
}

export const NODE_TYPES = { step: memo(StepNode) };

export function toFlowNodes(g: Graph, extra?: (n: GraphNode) => Partial<FlowData>, selected?: string | null): FlowNode[] {
  return g.nodes.map((n) => ({ id: n.id, type: "step", position: { x: n.x ?? 0, y: n.y ?? 0 }, selected: n.id === selected, data: { node: n, ...extra?.(n) } }));
}

export function toFlowEdges(g: Graph, style?: (e: { from: string; to: string }) => CSSProperties | undefined): Edge[] {
  return g.edges.map((e) => ({ id: `${e.from}->${e.to}`, source: e.from, target: e.to, style: style?.(e) }));
}
