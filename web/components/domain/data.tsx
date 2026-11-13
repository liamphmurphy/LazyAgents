import type { ReactNode } from "react";
import { cx } from "@/lib/cx";

export function ProgressBar({
  value,
  color = "var(--accent)",
  height = 4,
  label,
}: {
  /** 0..1 */
  value: number;
  color?: string;
  height?: 3 | 4;
  label?: string;
}) {
  const pct = Math.round(Math.min(1, Math.max(0, value)) * 100);
  return (
    <span
      role="progressbar"
      aria-valuenow={pct}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={label}
      className="block overflow-hidden rounded-[2px] bg-track"
      style={{ height }}
    >
      <span className="block h-full" style={{ width: `${pct}%`, background: color }} />
    </span>
  );
}

export type StackedSegment = { value: number; color: string; label: string };

/** Segments ordered done → review → progress → todo/backlog, 2px gaps. */
export function StackedBar({ segments, height = 6 }: { segments: StackedSegment[]; height?: 6 | 8 }) {
  const total = segments.reduce((n, s) => n + s.value, 0);
  const summary = segments.map((s) => `${s.value} ${s.label}`).join(", ");
  return (
    <span role="img" aria-label={summary} className="flex gap-0.5 overflow-hidden rounded-[3px]" style={{ height }}>
      {total === 0 ? (
        <span className="flex-1 bg-track" />
      ) : (
        segments.filter((s) => s.value > 0).map((s) => <span key={s.label} style={{ flex: s.value, background: s.color }} />)
      )}
    </span>
  );
}

/** Property row in detail panels: 88px tertiary key, value with leading glyph. */
export function PropertyRow({
  label,
  children,
  keyWidth = 88,
  onClick,
}: {
  label: string;
  children: ReactNode;
  keyWidth?: number;
  onClick?: () => void;
}) {
  const inner = (
    <>
      <span className="flex-none text-13 text-fg-tertiary" style={{ width: keyWidth }}>
        {label}
      </span>
      <span className="flex min-w-0 items-center gap-2 text-13 text-fg">{children}</span>
    </>
  );
  const cls = "flex min-h-[34px] w-full items-center gap-2.5 rounded-md px-2 text-left";
  return onClick ? (
    <button type="button" onClick={onClick} className={cx(cls, "hover:bg-hover")}>
      {inner}
    </button>
  ) : (
    <div className={cls}>{inner}</div>
  );
}

export function KpiTile({
  label,
  value,
  context,
  visual,
}: {
  label: string;
  value: ReactNode;
  context?: ReactNode;
  visual?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2 rounded-xl border border-border bg-surface px-[18px] py-4">
      <span className="text-13 text-fg-secondary">{label}</span>
      <div className="flex flex-wrap items-baseline gap-x-2">
        <span className="text-28 font-semibold leading-none tracking-[-0.02em] text-fg">{value}</span>
        {context && <span className="text-12 text-fg-tertiary">{context}</span>}
      </div>
      {visual}
    </div>
  );
}

/** Label/value stat used on run and agent cards. */
export function Stat({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-12 text-fg-tertiary">{label}</span>
      <span className="font-mono text-13 text-fg">{value}</span>
    </div>
  );
}
