import type { ComponentProps, ReactNode } from "react";
import { cx } from "@/lib/cx";
import { Icon, type IconName } from "./icon";

export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cx(
        "inline-block min-w-[18px] rounded-xs border border-kbd-border px-[5px] py-px text-center font-mono text-11 text-fg-secondary",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

/** Card: surface, 1px border, radius 12. Optional header with a right-side action. */
export function Card({
  title,
  action,
  children,
  className,
  bodyClassName,
  as: Tag = "section",
}: {
  title?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
  as?: "section" | "div" | "article";
}) {
  return (
    <Tag className={cx("rounded-xl border border-border bg-surface", className)}>
      {title && (
        <div className="flex items-center justify-between gap-3 border-b border-border-subtle px-[18px] py-3.5">
          <h2 className="type-heading text-fg">{title}</h2>
          {action}
        </div>
      )}
      <div className={cx("px-[18px] py-3.5", bodyClassName)}>{children}</div>
    </Tag>
  );
}

/** Two-cell filter chip: tertiary key | value. */
export function FilterChip({ label, value, ...rest }: { label: string; value: ReactNode } & ComponentProps<"button">) {
  return (
    <button
      type="button"
      className="inline-flex h-[26px] items-center overflow-hidden rounded-sm border border-border-strong text-12 hover:bg-hover"
      {...rest}
    >
      <span className="flex h-full items-center border-r border-border-strong px-2 text-fg-tertiary">{label}</span>
      <span className="px-2 text-fg-2">{value}</span>
    </button>
  );
}

export function AddFilterChip(props: ComponentProps<"button">) {
  return (
    <button
      type="button"
      className="inline-flex h-[26px] items-center gap-1.5 rounded-sm border border-dashed border-border-strong px-2.5 text-12 text-fg-secondary hover:bg-hover"
      {...props}
    >
      <Icon name="plus" size={12} strokeWidth={1.8} />
      Filter
    </button>
  );
}

export function Skeleton({ className, style }: { className?: string; style?: React.CSSProperties }) {
  return <span aria-hidden className={cx("block rounded-xs bg-track animate-skeleton", className)} style={style} />;
}

/**
 * What a screen or section shows when there's nothing in it yet: an icon, one heading, a line on
 * how things get here, and the action that adds the first one. `compact` fits a tray, card or
 * panel section. A one-line hint inside a side panel ("No headings yet.") stays plain text.
 */
export function EmptyState({
  icon,
  title,
  children,
  actions,
  className,
  compact,
}: {
  icon: IconName;
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
  className?: string;
  compact?: boolean;
}) {
  return (
    <div className={cx("flex flex-col items-center gap-2.5 px-4 text-center", compact ? "py-6" : "py-12", className)}>
      <span aria-hidden className={cx("dusk-tile mb-1 grid place-items-center rounded-lg border", compact ? "size-8" : "size-10")}>
        <Icon name={icon} size={compact ? 15 : 18} />
      </span>
      <h3 className="type-heading text-fg">{title}</h3>
      {children && <p className="max-w-[360px] text-13 text-fg-secondary">{children}</p>}
      {actions && <div className="mt-2 flex flex-wrap justify-center gap-2">{actions}</div>}
    </div>
  );
}

/**
 * Human-action banner. Amber = a person can act (approval, dispatch, setup).
 * Red = something is broken (runner offline, load error).
 */
export function Banner({
  tone = "human",
  lead,
  title,
  children,
  actions,
}: {
  tone?: "human" | "blocked";
  lead?: ReactNode;
  title: ReactNode;
  children?: ReactNode;
  actions?: ReactNode;
}) {
  const human = tone === "human";
  return (
    <div
      role={human ? "status" : "alert"}
      className="flex flex-wrap items-center gap-3.5 rounded-xl border px-4 py-3"
      style={{
        borderColor: `color-mix(in srgb, var(${human ? "--state-human" : "--state-blocked"}) 25%, transparent)`,
        background: `color-mix(in srgb, var(${human ? "--state-human" : "--state-blocked"}) 6%, transparent)`,
      }}
    >
      {lead}
      <div className="flex min-w-[220px] flex-1 flex-col gap-0.5">
        <span className={cx("font-semibold", human ? "text-pill-human-fg" : "text-pill-blocked-fg")}>{title}</span>
        {children && <span className="prose-code text-13 text-fg-2">{children}</span>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Field({
  label,
  hint,
  htmlFor,
  children,
}: {
  label: string;
  hint?: ReactNode;
  htmlFor?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={htmlFor} className="text-13 font-medium text-fg-2">
        {label}
      </label>
      {children}
      {hint && <span id={htmlFor ? `${htmlFor}-hint` : undefined} className="text-12 text-fg-tertiary">{hint}</span>}
    </div>
  );
}

const inputBase =
  "rounded-md border border-border-strong bg-surface text-fg placeholder:text-fg-tertiary focus:border-accent focus:outline-none";

/** Full width unless the caller sets a width (w-*, flex-1, min-w-*). */
function widthOf(className?: string) {
  return className && /(^|\s)(w-|flex-1|min-w-)/.test(className) ? "" : "w-full";
}

export function TextInput({ mono, prefix, className, ...rest }: { mono?: boolean; prefix?: string } & ComponentProps<"input">) {
  const font = mono ? "font-mono text-13" : "text-13";
  if (prefix) {
    return (
      <span className={cx(inputBase, widthOf(className), "flex h-[30px] items-center overflow-hidden focus-within:border-accent", className)}>
        <span className="pl-2.5 font-mono text-13 text-fg-tertiary">{prefix}</span>
        <input className={cx("h-full min-w-0 flex-1 bg-transparent pl-px pr-2.5 outline-none", font)} {...rest} />
      </span>
    );
  }
  return <input className={cx(inputBase, widthOf(className), "h-[30px] px-2.5", font, className)} {...rest} />;
}

export function TextArea({ mono, className, ...rest }: { mono?: boolean } & ComponentProps<"textarea">) {
  return (
    <textarea
      className={cx(
        inputBase,
        widthOf(className),
        "px-2.5 py-2",
        mono ? "bg-surface-sunken font-mono text-13 leading-relaxed" : "type-reading",
        className,
      )}
      {...rest}
    />
  );
}
