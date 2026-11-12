"use client";

// <DesignSvg> and the op renderer (ADR-designs §3.8): React elements for the
// display list in layout.ts, used by the editor canvas, Home thumbnails and
// (later) review, Present and the Design card. Text is one <text> per
// measured piece with kerning off, so it lands where layout.ts measured it.
import { memo, useEffect, useMemo, useState, type ReactNode } from "react";
import { FAMILY, fontUrl } from "./measure";
import { makeLayout, type LaidPage, type Op, type ResolvedPaint } from "./layout";
import type { Design, Page } from "./schema";

const WEIGHTS = [400, 500, 600, 700];
let loading: Promise<void> | undefined;

/** Registers the eight Inter faces once; later calls share the promise. */
export function loadDesignFonts(): Promise<void> {
  if (loading) return loading;
  if (typeof document === "undefined" || typeof FontFace === "undefined") return Promise.resolve();
  loading = Promise.all(
    WEIGHTS.flatMap((w) =>
      [false, true].map(async (italic) => {
        const f = new FontFace(FAMILY, `url(${fontUrl(w, italic)}) format("woff2")`, { weight: String(w), style: italic ? "italic" : "normal" });
        document.fonts.add(await f.load());
      }),
    ),
  ).then(() => undefined);
  loading.catch(() => (loading = undefined));
  return loading;
}

/** True once Inter has loaded (layout doesn't wait: it measures from the table). */
export function useDesignFonts(): boolean {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let live = true;
    loadDesignFonts()
      .catch(() => undefined)
      .then(() => live && setReady(true));
    return () => {
      live = false;
    };
  }, []);
  return ready;
}

export const designAssetHref = (canvasId: string, assetId: string) => `/api/v1/designs/${canvasId}/assets/${assetId}`;

export const TEXT_STYLE = { fontKerning: "none", fontFeatureSettings: '"kern" 0', textRendering: "geometricPrecision" } as const;

function fillOf(p: ResolvedPaint | undefined, id: string, defs: ReactNode[]): { fill: string; fillOpacity?: number } {
  if (!p) return { fill: "none" };
  if (typeof p === "string") return p.length === 9 ? { fill: p.slice(0, 7), fillOpacity: parseInt(p.slice(7), 16) / 255 } : { fill: p };
  const a = (p.angle * Math.PI) / 180;
  defs.push(
    <linearGradient key={id} id={id} x1={0.5 - Math.cos(a) / 2} y1={0.5 - Math.sin(a) / 2} x2={0.5 + Math.cos(a) / 2} y2={0.5 + Math.sin(a) / 2}>
      <stop offset="0" stopColor={p.stops[0]} />
      <stop offset="1" stopColor={p.stops[1]} />
    </linearGradient>,
  );
  return { fill: `url(#${id})` };
}

function strokeOf(o: { stroke?: string; sw: number; dash?: string; cap?: string; join?: string }) {
  if (!o.stroke || !o.sw) return {};
  const s = o.stroke.length === 9 ? { stroke: o.stroke.slice(0, 7), strokeOpacity: parseInt(o.stroke.slice(7), 16) / 255 } : { stroke: o.stroke };
  return { ...s, strokeWidth: o.sw, strokeDasharray: o.dash, strokeLinecap: o.cap as "round" | undefined, strokeLinejoin: o.join as "round" | undefined };
}

/** Ops as nested SVG elements (clip and rotate open groups). uid keeps defs ids unique per renderer. */
export function renderOps(ops: Op[], uid: string, assetUrl: (asset: string) => string): ReactNode[] {
  const root: ReactNode[] = [];
  const stack: { kids: ReactNode[]; make: (kids: ReactNode[]) => ReactNode }[] = [];
  let kids = root;
  ops.forEach((o, i) => {
    const key = i;
    const op = "opacity" in o && o.opacity !== undefined && o.opacity < 1 ? o.opacity : undefined;
    switch (o.op) {
      case "rect": {
        const defs: ReactNode[] = [];
        const f = fillOf(o.fill, `${uid}g${i}`, defs);
        kids.push(defs.length ? <g key={key}><defs>{defs}</defs><rect x={o.x} y={o.y} width={Math.max(0, o.w)} height={Math.max(0, o.h)} rx={o.r || undefined} {...f} {...strokeOf(o)} opacity={op} /></g> : <rect key={key} x={o.x} y={o.y} width={Math.max(0, o.w)} height={Math.max(0, o.h)} rx={o.r || undefined} {...f} {...strokeOf(o)} opacity={op} />);
        break;
      }
      case "ellipse": {
        const defs: ReactNode[] = [];
        const f = fillOf(o.fill, `${uid}g${i}`, defs);
        kids.push(<g key={key}>{defs.length > 0 && <defs>{defs}</defs>}<ellipse cx={o.x + o.w / 2} cy={o.y + o.h / 2} rx={Math.max(0, o.w / 2)} ry={Math.max(0, o.h / 2)} {...f} {...strokeOf(o)} opacity={op} /></g>);
        break;
      }
      case "path": {
        const defs: ReactNode[] = [];
        const f = fillOf(o.fill, `${uid}g${i}`, defs);
        kids.push(<g key={key}>{defs.length > 0 && <defs>{defs}</defs>}<path d={o.d} {...f} {...strokeOf(o)} opacity={op} /></g>);
        break;
      }
      case "polyline":
        kids.push(<polyline key={key} points={o.points.map((p) => p.join(",")).join(" ")} fill="none" {...strokeOf(o)} opacity={op} />);
        break;
      case "image": {
        const sx = o.w / o.src.w, sy = o.h / o.src.h;
        const cid = `${uid}c${i}`;
        kids.push(
          <g key={key} opacity={op}>
            <defs><clipPath id={cid}><rect x={o.x} y={o.y} width={o.w} height={o.h} rx={o.r || undefined} /></clipPath></defs>
            <g clipPath={`url(#${cid})`}>
              <image href={assetUrl(o.asset)} x={o.x - o.src.x * sx} y={o.y - o.src.y * sy} width={o.iw * sx} height={o.ih * sy} preserveAspectRatio="none"><title>{o.alt}</title></image>
            </g>
          </g>,
        );
        break;
      }
      case "text": {
        const f = fillOf(o.fill, "", []);
        kids.push(<text key={key} x={o.x} y={o.y} fontFamily={`"${FAMILY}", Inter, sans-serif`} fontSize={o.size} fontWeight={o.weight} fontStyle={o.italic ? "italic" : undefined} {...f} opacity={op} style={TEXT_STYLE} xmlSpace="preserve">{o.text}</text>);
        break;
      }
      case "clip": {
        const cid = `${uid}c${i}`;
        const box = o;
        stack.push({ kids, make: (inner) => <g key={key}><defs><clipPath id={cid}><rect x={box.x} y={box.y} width={box.w} height={box.h} rx={box.r || undefined} /></clipPath></defs><g clipPath={`url(#${cid})`}>{inner}</g></g> });
        kids = [];
        break;
      }
      case "rotate": {
        const t = `rotate(${o.deg} ${o.cx} ${o.cy})`;
        stack.push({ kids, make: (inner) => <g key={key} transform={t}>{inner}</g> });
        kids = [];
        break;
      }
      case "unclip":
      case "unrotate": {
        const top = stack.pop();
        if (!top) break;
        const inner = kids;
        kids = top.kids;
        kids.push(top.make(inner));
        break;
      }
    }
  });
  while (stack.length) {
    const top = stack.pop()!;
    const inner = kids;
    kids = top.kids;
    kids.push(top.make(inner));
  }
  return root;
}

/** One top-level node's elements; memoised on its ops so panning never re-renders it. */
export const LaidNodeSvg = memo(function LaidNodeSvg({ ops, uid, assetUrl }: { ops: Op[]; uid: string; assetUrl: (a: string) => string }) {
  return <g>{renderOps(ops, uid, assetUrl)}</g>;
});

/** A laid page's content, optionally only the nodes in `visible` (viewport culling). */
export const LaidPageSvg = memo(function LaidPageSvg({ laid, uid, assetUrl, visible }: { laid: LaidPage; uid: string; assetUrl: (a: string) => string; visible?: ReadonlySet<string> }) {
  return (
    <>
      {laid.nodes.map((n) => (!visible || visible.has(n.id) ? <LaidNodeSvg key={n.id} ops={n.ops} uid={`${uid}${n.id}`} assetUrl={assetUrl} /> : null))}
      <LaidNodeSvg ops={laid.connectors} uid={`${uid}conn`} assetUrl={assetUrl} />
    </>
  );
});

/** A page (or one node of it) as a static picture fitted to its box: Home thumbnails, cards. */
export function DesignSvg({ design, page, node, assetUrl, className, title, pad = 0 }: { design: Design; page?: Page; node?: string; assetUrl: (a: string) => string; className?: string; title?: string; pad?: number }) {
  const p = page ?? design.pages[0];
  const laid = useMemo(() => (p ? makeLayout(design).layoutPage(p) : undefined), [design, p]);
  const uid = useMemo(() => `ds${Math.random().toString(36).slice(2, 8)}`, []);
  if (!laid) return null;
  const only = node ? laid.nodes.find((n) => n.id === node) : undefined;
  const box = only?.box ?? laid.box;
  const w = Math.max(1, box.w) + 2 * pad, h = Math.max(1, box.h) + 2 * pad;
  return (
    <svg viewBox={`${box.x - pad} ${box.y - pad} ${w} ${h}`} className={className} role="img" aria-label={title} preserveAspectRatio="xMidYMid meet">
      {laid.background && <rect x={box.x - pad} y={box.y - pad} width={w} height={h} fill={laid.background} />}
      {only ? <LaidNodeSvg ops={only.ops} uid={uid} assetUrl={assetUrl} /> : <LaidPageSvg laid={laid} uid={uid} assetUrl={assetUrl} />}
    </svg>
  );
}
