"use client";

import { useEffect, useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import type { Zoom } from "@/lib/mockup";
import { Flag, type PillTone } from "./labels";

export type ReviewState = Schemas["MockupReviewState"];

const REVIEW: Record<ReviewState, { tone: PillTone; label: string }> = {
  pending: { tone: "human", label: "Needs review" },
  approved: { tone: "done", label: "Approved" },
  changes_requested: { tone: "blocked", label: "Changes requested" },
};

export function reviewLabel(state: ReviewState) {
  return REVIEW[state].label;
}

export function ReviewChip({ state }: { state: ReviewState }) {
  return <Flag tone={REVIEW[state].tone}>{REVIEW[state].label}</Flag>;
}

const checker =
  "bg-surface-sunken [background-image:conic-gradient(var(--border-subtle)_25%,transparent_0_50%,var(--border-subtle)_0_75%,transparent_0)] [background-size:16px_16px]";

/**
 * Renders SVG source through <img> with a data: URL. The markup is never
 * injected into the DOM, so scripts and external loads can't run in the page.
 * `zoom` fit scales to the container; 100/200 use the SVG's own size and scroll.
 */
export function MockupImage({
  source,
  alt,
  zoom = "fit",
  className,
  onLoadError,
}: {
  source: string;
  alt: string;
  zoom?: Zoom;
  className?: string;
  onLoadError?: () => void;
}) {
  const [url, setUrl] = useState<string | null>(null);
  const [natural, setNatural] = useState<{ w: number; h: number } | null>(null);
  const [broken, setBroken] = useState(false);

  // A data: URL, not blob:. A blob URL carries the app's origin, so "Open
  // image in new tab" would render the SVG top-level on it; data: URLs get an
  // opaque origin and browsers refuse top-level navigation to them.
  useEffect(() => {
    const bytes = new TextEncoder().encode(source);
    let bin = "";
    for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    const u = "data:image/svg+xml;base64," + btoa(bin);
    let live = true;
    queueMicrotask(() => live && setUrl(u));
    return () => {
      live = false;
    };
  }, [source]);

  const scale = zoom === "200" ? 2 : 1;
  return (
    <div className={cx("overflow-auto rounded-xl border border-border", checker, className)}>
      {url && (
        // eslint-disable-next-line @next/next/no-img-element -- data: URL of user SVG source
        <img
          src={url}
          alt={alt}
          onLoad={(e) => {
            setBroken(false);
            const i = e.currentTarget;
            setNatural({ w: i.naturalWidth, h: i.naturalHeight });
          }}
          onError={() => {
            setBroken(true);
            onLoadError?.();
          }}
          className={cx("block", zoom === "fit" ? "mx-auto h-auto max-h-full w-full max-w-full object-contain" : "max-w-none")}
          style={zoom !== "fit" && natural ? { width: natural.w * scale, height: natural.h * scale } : undefined}
        />
      )}
      {broken && <p className="p-4 text-xs text-pill-blocked-fg">The browser couldn&apos;t render this SVG.</p>}
    </div>
  );
}
