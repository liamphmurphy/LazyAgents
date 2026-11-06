import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { Dot } from "./glyphs";

export type PillTone = "human" | "blocked" | "review" | "done" | "neutral";

const tone = (t: PillTone) => `bg-pill-${t}-bg text-pill-${t}-fg`;
// Tailwind needs the full class names present in source:
// bg-pill-human-bg text-pill-human-fg bg-pill-blocked-bg text-pill-blocked-fg
// bg-pill-review-bg text-pill-review-fg bg-pill-done-bg text-pill-done-fg
// bg-pill-neutral-bg text-pill-neutral-fg

/** Status pill: full pill, 24px, 6px leading dot ("Running", "On track"). */
export function Pill({ tone: t, children, dot = true }: { tone: PillTone; children: ReactNode; dot?: boolean }) {
  return (
    <span className={cx("inline-flex h-6 flex-none items-center gap-1.5 whitespace-nowrap rounded-full px-[9px] text-xs font-medium", tone(t))}>
      {dot && <span aria-hidden className="size-1.5 rounded-full bg-current" />}
      {children}
    </span>
  );
}

/** Compact pill used inside cards. */
export function SmallPill({ tone: t, children }: { tone: PillTone; children: ReactNode }) {
  return (
    <span className={cx("flex-none whitespace-nowrap rounded-[10px] px-2 py-0.5 text-[11.5px] font-medium", tone(t))}>{children}</span>
  );
}

/** Inline flag on rows: "Needs approval", "Blocked", "PR #482". Radius 5. */
export function Flag({ tone: t, children }: { tone: PillTone; children: ReactNode }) {
  return (
    <span className={cx("flex-none whitespace-nowrap rounded-[5px] px-[7px] py-px text-[11.5px] font-medium", tone(t))}>{children}</span>
  );
}

/** Mono machine-value chip: models, scopes, tools. */
export function MonoChip({ children, variant = "fill" }: { children: ReactNode; variant?: "fill" | "outline" }) {
  return (
    <span
      className={cx(
        "whitespace-nowrap rounded-[5px] px-[7px] py-0.5 font-mono text-[11px]",
        variant === "fill" ? "bg-selected text-fg-2" : "border border-track text-fg-tertiary",
      )}
    >
      {children}
    </span>
  );
}

export type DocType = "prd" | "spec" | "adr" | "report" | "design" | "note" | "mockup";
const DOC_CLASS: Record<DocType, string> = {
  note: "bg-doc-note-bg text-doc-note",
  prd: "bg-doc-prd-bg text-doc-prd",
  spec: "bg-doc-spec-bg text-doc-spec",
  adr: "bg-doc-adr-bg text-doc-adr",
  report: "bg-doc-report-bg text-doc-report",
  design: "bg-doc-design-bg text-doc-design",
  mockup: "bg-doc-mockup-bg text-doc-mockup",
};

export function DocTypeTag({ type, className }: { type: DocType; className?: string }) {
  return <span className={cx("flex-none rounded-xs px-1.5 py-0.5 type-micro", DOC_CLASS[type], className)}>{type}</span>;
}

export function LabelChip({ name, color }: { name: string; color: string }) {
  return (
    <span className="inline-flex h-[22px] items-center gap-1.5 whitespace-nowrap rounded-full border border-border-strong px-2 text-[11.5px] text-fg-secondary">
      <span aria-hidden className="size-1.5 rounded-full" style={{ background: color }} />
      {name}
    </span>
  );
}

export type ScopeModeValue = "allowed" | "requires_approval" | "denied";

/** Scope mode: state dot + label. Always shown alongside an EnforcementChip, never replaced by it. */
export function ScopeMode({ mode }: { mode: ScopeModeValue }) {
  const m = {
    allowed: { dot: "done" as const, label: "Allowed", cls: "text-fg-2" },
    requires_approval: { dot: "human" as const, label: "Requires approval", cls: "text-pill-human-fg" },
    denied: { dot: "blocked" as const, label: "Denied", cls: "text-pill-blocked-fg" },
  }[mode];
  return (
    <span className={cx("inline-flex items-center gap-2 whitespace-nowrap text-[12.5px]", m.cls)}>
      <Dot tone={m.dot} />
      {m.label}
    </span>
  );
}

export type Enforcement = "credential" | "brokered" | "harness" | "none";

/** How a scope is enforced. Harness-gated (and none) are weaker, so they get the outlined chip. */
export function EnforcementChip({ level }: { level: Enforcement }) {
  return <MonoChip variant={level === "harness" || level === "none" ? "outline" : "fill"}>{level}</MonoChip>;
}
