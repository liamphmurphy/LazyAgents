import type { CSSProperties } from "react";
import { cx } from "@/lib/cx";

export type TicketStatus = "backlog" | "todo" | "in_progress" | "in_review" | "done";
export type GlyphState = TicketStatus | "human" | "blocked";
export type Priority = "none" | "low" | "medium" | "high" | "urgent";

export const STATUS_LABEL: Record<GlyphState, string> = {
  backlog: "Backlog",
  todo: "Todo",
  in_progress: "In progress",
  in_review: "In review",
  done: "Done",
  human: "Needs human",
  blocked: "Blocked",
};

const pie = (token: string, pct: number, surface: string): CSSProperties => ({
  border: `1.5px solid var(${token})`,
  background: `conic-gradient(var(${token}) 0 ${pct}%, transparent 0)`,
  boxShadow: `inset 0 0 0 2px ${surface}`,
});

/**
 * Workflow-state glyph. Shape carries the state independently of color
 * (DESIGN.md §6): dashed ring, ring, half pie, three-quarter pie, filled.
 * `surface` is the background the glyph sits on, used for the pie gap.
 */
export function StatusGlyph({
  state,
  size = 14,
  surface = "var(--bg)",
  label = true,
}: {
  state: GlyphState;
  size?: number;
  surface?: string;
  /** Announce the state to screen readers. Turn off when a visible label sits next to it. */
  label?: boolean;
}) {
  const styles: Record<GlyphState, CSSProperties> = {
    backlog: { border: "1.5px dashed var(--state-backlog)" },
    todo: { border: "1.5px solid var(--state-todo)" },
    in_progress: pie("--state-progress", 50, surface),
    in_review: pie("--state-review", 75, surface),
    done: { background: "var(--state-done)" },
    human: { background: "var(--state-human)" },
    blocked: { background: "var(--state-blocked)" },
  };
  return (
    <span
      role={label ? "img" : undefined}
      aria-label={label ? STATUS_LABEL[state] : undefined}
      aria-hidden={label ? undefined : true}
      className="inline-block flex-none rounded-full"
      style={{ width: size, height: size, ...styles[state] }}
    />
  );
}

export function PriorityGlyph({ priority, label = true }: { priority: Priority; label?: boolean }) {
  const aria = label ? { role: "img" as const, "aria-label": `${priority[0].toUpperCase()}${priority.slice(1)} priority` } : { "aria-hidden": true };
  if (priority === "urgent") {
    return (
      <span
        {...aria}
        className="grid size-4 flex-none place-items-center rounded-xs bg-priority-urgent text-[11px] font-bold leading-none text-[#0B0B0D]"
      >
        !
      </span>
    );
  }
  const lit = { high: 3, medium: 2, low: 1, none: 0 }[priority];
  return (
    <span {...aria} className="flex size-4 flex-none items-end justify-center gap-0.5">
      {[6, 9, 12].map((h, i) => (
        <span key={h} className={cx("w-[3px] rounded-[1px]", i < lit ? "bg-fg-2" : "bg-priority-unlit")} style={{ height: h }} />
      ))}
    </span>
  );
}

/** 8px state dot, used by scope modes and live indicators. */
export function Dot({ tone, size = 8 }: { tone: "done" | "human" | "blocked" | "review" | "progress" | "accent" | "backlog"; size?: number }) {
  const v = tone === "accent" ? "--accent" : `--state-${tone}`;
  return <span aria-hidden className="inline-block flex-none rounded-full" style={{ width: size, height: size, background: `var(${v})` }} />;
}
