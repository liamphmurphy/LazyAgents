"use client";

import { useEffect, useMemo, useState } from "react";
import { FAMILY, loadSlideFonts, measure, resetMeasureCache } from "./fonts";
import { layoutSlide, type Laid, type Op } from "./layout";
import { H, W, slideText, type Deck, type Slide } from "./schema";

/** True once the slide fonts have loaded; layout before that would measure a fallback font. */
export function useSlideFonts(): boolean {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let live = true;
    loadSlideFonts()
      .catch(() => undefined)
      .then(() => {
        resetMeasureCache();
        if (live) setReady(true);
      });
    return () => {
      live = false;
    };
  }, []);
  return ready;
}

export const assetHref = (canvasId: string, assetId: string) => `/api/v1/slides/${canvasId}/assets/${assetId}`;

/** A slide's display list, recomputed only when the slide, theme or assets change. */
export function useLaidSlide(deck: Deck, slide: Slide, fontsReady: boolean): Laid {
  // fontsReady is a dependency: the layout must be redone once real metrics exist.
  return useMemo(() => layoutSlide(deck, slide, measure), [deck.theme, deck.assets, slide, fontsReady]); // eslint-disable-line react-hooks/exhaustive-deps
}

function rotate(o: Op) {
  return o.rot ? `rotate(${o.rot.deg} ${o.rot.cx} ${o.rot.cy})` : undefined;
}

function draw(o: Op, i: number, deck: Deck, assetUrl: (id: string) => string, uid: string) {
  const t = rotate(o);
  switch (o.op) {
    case "rect":
      return <rect key={i} x={o.x} y={o.y} width={o.w} height={o.h} rx={o.r} fill={o.fill ?? "none"} stroke={o.stroke} strokeWidth={o.stroke ? o.sw : undefined} transform={t} />;
    case "ellipse":
      return <ellipse key={i} cx={o.x + o.w / 2} cy={o.y + o.h / 2} rx={o.w / 2} ry={o.h / 2} fill={o.fill ?? "none"} stroke={o.stroke} strokeWidth={o.stroke ? o.sw : undefined} transform={t} />;
    case "line": {
      const id = `${uid}-arrow-${i}`;
      return (
        <g key={i} transform={t}>
          {(o.endArrow || o.startArrow) && (
            <defs>
              <marker id={id} viewBox="0 0 10 10" refX="8" refY="5" markerWidth="4" markerHeight="4" orient="auto-start-reverse">
                <path d="M0,0 L10,5 L0,10 z" fill={o.stroke} />
              </marker>
            </defs>
          )}
          <line x1={o.x1} y1={o.y1} x2={o.x2} y2={o.y2} stroke={o.stroke} strokeWidth={o.sw} markerEnd={o.endArrow ? `url(#${id})` : undefined} markerStart={o.startArrow ? `url(#${id})` : undefined} />
        </g>
      );
    }
    case "image": {
      const a = deck.assets[o.asset];
      const s = o.w / o.src.w;
      const clip = `${uid}-clip-${i}`;
      return (
        <g key={i} transform={t}>
          <clipPath id={clip}>
            <rect x={o.x} y={o.y} width={o.w} height={o.h} />
          </clipPath>
          <image clipPath={`url(#${clip})`} href={assetUrl(o.asset)} x={o.x - o.src.x * s} y={o.y - o.src.y * s} width={a.w * s} height={a.h * s} preserveAspectRatio="none">
            {o.alt && <title>{o.alt}</title>}
          </image>
        </g>
      );
    }
    case "highlight":
      return <rect key={i} x={o.x} y={o.y} width={o.w} height={o.h} fill={o.fill} transform={t} />;
    case "text":
      return (
        <text
          key={i}
          x={o.x}
          y={o.y}
          fontFamily={`"${FAMILY[o.slot]}"`}
          fontSize={o.size}
          fontWeight={o.style.includes("bold") ? 700 : 400}
          fontStyle={o.style.includes("italic") ? "italic" : "normal"}
          fill={o.color}
          textDecoration={[o.underline && "underline", o.strike && "line-through"].filter(Boolean).join(" ") || undefined}
          style={{ fontKerning: "none", whiteSpace: "pre" }}
          transform={t}
        >
          {o.text}
        </text>
      );
  }
}

let seq = 0;

/**
 * One slide drawn from the shared layout. It scales to its container's width;
 * marks outline elements (red/green in review, amber for overflow).
 */
export function SlideSvg({ deck, slide, assetUrl, className, label, marks, showOverflow, fontsReady, hideTextOf }: {
  deck: Deck;
  slide: Slide;
  assetUrl: (id: string) => string;
  className?: string;
  label?: string;
  marks?: Record<string, "added" | "removed" | "changed">;
  showOverflow?: boolean;
  fontsReady: boolean;
  /** An element whose text is being edited in place, so isn't drawn here. */
  hideTextOf?: string;
}) {
  const laid = useLaidSlide(deck, slide, fontsReady);
  const [uid] = useState(() => `slide${++seq}`);
  const outline = (id: string) => slide.elements.find((e) => e.id === id)?.frame;
  return (
    // geometricPrecision: unhinted glyph advances, so text is as wide as the layout measured at any scale.
    <svg viewBox={`0 0 ${W} ${H}`} className={className} textRendering="geometricPrecision" role="img" aria-label={label ?? (slideText(slide) || "Empty slide")} preserveAspectRatio="xMidYMid meet">
      {laid.ops.map((o, i) => (hideTextOf && (o.op === "text" || o.op === "highlight") && o.el === hideTextOf ? null : draw(o, i, deck, assetUrl, uid)))}
      {showOverflow &&
        [...laid.overflow].map((id) => {
          const f = outline(id);
          return f && <rect key={`o-${id}`} x={f.x} y={f.y} width={f.w} height={f.h} fill="none" stroke="#F59E5B" strokeWidth={6} strokeDasharray="24 14" pointerEvents="none" />;
        })}
      {marks &&
        Object.entries(marks).map(([id, kind]) => {
          const f = outline(id);
          return (
            f && (
              <rect key={`m-${id}`} x={f.x - 6} y={f.y - 6} width={f.w + 12} height={f.h + 12} fill="none" stroke={kind === "removed" ? "#EF6461" : "#4CB782"} strokeWidth={10} strokeDasharray={kind === "removed" ? "30 18" : undefined} pointerEvents="none" />
            )
          );
        })}
    </svg>
  );
}
