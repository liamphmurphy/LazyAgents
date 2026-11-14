import { cx } from "@/lib/cx";
import { Skeleton } from "./primitives";

/**
 * The loading language (DESIGN.md §5 States): content that's on its way is a skeleton shaped
 * like it, and a button that's working shows a Spinner beside its label. Skeletons stay
 * invisible for their first 150ms (animate-skeleton in globals.css), so a fast load never
 * flickers. Words like "Loading…" are only for screen readers.
 */

/** For buttons and inline progress only; a section that's loading is a skeleton. */
export function Spinner({ size = 12, className }: { size?: number; className?: string }) {
  return (
    <span
      aria-hidden
      className={cx("inline-block flex-none animate-spin rounded-full border-border-strong border-t-accent", size < 12 ? "border-[1.5px]" : "border-2", className)}
      style={{ width: size, height: size }}
    />
  );
}

const WIDTHS = ["w-2/3", "w-1/2", "w-5/6", "w-1/3"];

/** Stand-in lines of text while a section loads; announced once as "Loading". */
export function LoadingLines({ lines = 2, className, label = "Loading" }: { lines?: number; className?: string; label?: string }) {
  return (
    <div role="status" className={cx("flex flex-col gap-2 py-1", className)}>
      {Array.from({ length: lines }, (_, i) => <Skeleton key={i} className={cx("h-3", WIDTHS[i % WIDTHS.length])} />)}
      <span className="sr-only">{label}</span>
    </div>
  );
}

/** A stand-in for one short value inside a line of text (a count, a subtitle). */
export function LoadingText({ className = "w-20", label = "Loading" }: { className?: string; label?: string }) {
  return (
    <span role="status" className="inline-flex align-middle">
      <Skeleton className={cx("inline-block h-[0.75em]", className)} />
      <span className="sr-only">{label}</span>
    </span>
  );
}
