"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { cx } from "@/lib/cx";
import { sparkPath } from "@/lib/finance";

const still = () => typeof window === "undefined" || window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/**
 * A number that counts from its last value to the new one. The animated digits live in an
 * aria-hidden span React never renders into (it's written from script), and the real value
 * is beside it for screen readers.
 */
export function CountUp({ value, format, duration = 900, className }: { value: number; format: (n: number) => string; duration?: number; className?: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  const from = useRef(0);
  const fmt = useRef(format);
  useLayoutEffect(() => {
    fmt.current = format;
  });
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const start = from.current;
    from.current = value;
    if (still() || start === value) {
      el.textContent = fmt.current(value);
      return;
    }
    let raf = 0;
    const t0 = performance.now();
    const tick = (t: number) => {
      const p = Math.min(1, (t - t0) / duration);
      el.textContent = fmt.current(start + (value - start) * (1 - Math.pow(1 - p, 3)));
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    el.textContent = fmt.current(start);
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [value, duration]);
  return (
    <span className={className}>
      <span ref={ref} aria-hidden />
      <span className="sr-only">{format(value)}</span>
    </span>
  );
}

/** A sparkline that draws itself in; re-keyed by its data so new numbers draw again. */
export function Sparkline({ values, color, className }: { values: number[]; color: string; className?: string }) {
  const w = 84;
  const h = 26;
  const d = sparkPath(values, w, h);
  if (!d) return <span className={cx("inline-block", className)} style={{ width: w, height: h }} aria-hidden />;
  const last = d.split("L").at(-1)!.replace("M", "").split(",").map(Number);
  return (
    <svg key={d} viewBox={`0 0 ${w} ${h}`} width={w} height={h} className={cx("overflow-visible", className)} aria-hidden>
      <path d={d} pathLength={1} className="fin-spark" fill="none" stroke={color} strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round" />
      <circle cx={last[0]} cy={last[1]} r={2.2} fill={color} className="fin-spark-dot" />
    </svg>
  );
}

export type ToastMessage = { id: number; text: React.ReactNode; tone?: "error" };

/** One message at a time along the bottom, for what a click just did. */
export function useToast() {
  const [toast, setToast] = useState<ToastMessage | null>(null);
  useEffect(() => {
    if (!toast) return;
    const t = window.setTimeout(() => setToast((cur) => (cur?.id === toast.id ? null : cur)), 3600);
    return () => window.clearTimeout(t);
  }, [toast]);
  const show = (text: React.ReactNode, tone?: "error") => setToast({ id: Date.now(), text, tone });
  return { toast, show };
}

export function Toast({ toast }: { toast: ToastMessage | null }) {
  return (
    <div aria-live="polite" className="pointer-events-none fixed bottom-5 left-1/2 z-50">
      {toast && (
        <div
          key={toast.id}
          className={cx(
            "fin-toast -translate-x-1/2 whitespace-nowrap rounded-lg border bg-surface-raised px-3.5 py-2 text-13 shadow-[var(--shadow-float)]",
            toast.tone === "error" ? "border-pill-blocked-fg/40 text-pill-blocked-fg" : "border-border-strong text-fg-2",
          )}
        >
          {toast.text}
        </div>
      )}
    </div>
  );
}
