import Link from "next/link";
import type { ComponentProps, ReactNode } from "react";
import { cx } from "@/lib/cx";
import { Icon, type IconName } from "./icon";

export type ButtonVariant = "primary" | "secondary" | "outline" | "ghost" | "approve" | "approve-outline";

const base =
  "inline-flex h-[30px] items-center gap-1.5 whitespace-nowrap rounded-md px-3 text-[13px] transition-colors disabled:cursor-not-allowed disabled:opacity-50";

const variants: Record<ButtonVariant, string> = {
  primary: "bg-accent font-semibold text-on-accent hover:brightness-110",
  secondary: "border border-btn-secondary-border bg-btn-secondary font-medium text-fg hover:bg-hover",
  outline: "border border-border-strong bg-btn-outline text-fg-2 hover:bg-hover",
  ghost: "px-2.5 text-fg-2 hover:bg-hover",
  // Approval surfaces only: amber means a person is acting (DESIGN.md §1).
  approve: "bg-state-human text-[12.5px] font-semibold text-approve-fg hover:brightness-105",
  "approve-outline": "border border-pill-human-bg text-[12.5px] text-pill-human-fg hover:bg-pill-human-bg",
};

export function buttonClass(variant: ButtonVariant = "secondary", className?: string) {
  return cx(base, variants[variant], className);
}

type Extra = { variant?: ButtonVariant; icon?: IconName; hint?: string; children?: ReactNode };

function Inner({ icon, hint, children }: Pick<Extra, "icon" | "hint" | "children">) {
  return (
    <>
      {icon && <Icon name={icon} size={14} strokeWidth={icon === "plus" ? 2 : 1.6} />}
      {children}
      {hint && <span className="ml-0.5 font-mono text-[11px] opacity-60">{hint}</span>}
    </>
  );
}

export function Button({ variant, icon, hint, children, className, type = "button", ...rest }: Extra & ComponentProps<"button">) {
  return (
    <button type={type} className={buttonClass(variant, className)} {...rest}>
      <Inner icon={icon} hint={hint}>{children}</Inner>
    </button>
  );
}

export function ButtonLink({ variant, icon, hint, children, className, ...rest }: Extra & ComponentProps<typeof Link>) {
  return (
    <Link className={buttonClass(variant, className)} {...rest}>
      <Inner icon={icon} hint={hint}>{children}</Inner>
    </Link>
  );
}

/** Icon-only button. `label` is required: it becomes the aria-label. */
export function IconButton({
  icon,
  label,
  size = 28,
  bordered,
  className,
  type = "button",
  ...rest
}: { icon: IconName; label: string; size?: number; bordered?: boolean } & Omit<ComponentProps<"button">, "children">) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      className={cx(
        "grid flex-none place-items-center rounded-sm text-fg-secondary transition-colors hover:bg-hover hover:text-fg",
        bordered && "border border-border-strong",
        className,
      )}
      style={{ width: size, height: size }}
      {...rest}
    >
      <Icon name={icon} size={15} />
    </button>
  );
}
