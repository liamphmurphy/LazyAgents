"use client";

import { DropdownMenu } from "radix-ui";
import { useMemo, type KeyboardEvent } from "react";
import { cx } from "@/lib/cx";
import { H, LAYOUTS, NEW_LAYOUTS, W, placeholders, resolveColor, type Element, type Layout, type Theme } from "@/lib/slides/schema";

/**
 * A layout drawn from its placeholders in the deck's theme: headings as bars
 * in the heading colour, bodies as bullet lines, image slots as boxes.
 */
export function LayoutThumb({ layout, theme }: { layout: Layout; theme: Theme }) {
  const els = useMemo(() => placeholders(layout), [layout]);
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="block aspect-video w-full" aria-hidden>
      <rect width={W} height={H} fill={theme.colors.background} />
      {els.map((e, i) => <Mark key={i} e={e} theme={theme} />)}
    </svg>
  );
}

function Mark({ e, theme }: { e: Element; theme: Theme }) {
  const f = e.frame;
  if (e.type !== "text") return null;
  if (e.placeholder === "image") {
    const full = f.w >= W && f.h >= H;
    const cx = f.x + f.w / 2, cy = f.y + f.h / 2;
    return (
      <g>
        <rect x={f.x} y={f.y} width={f.w} height={f.h} rx={full ? 0 : 24} fill={full ? theme.colors.muted : theme.colors.surface} />
        <path d={`M${cx - 130},${cy + 80} l80,-110 l60,80 l50,-50 l70,80 z`} fill={full ? theme.colors.background : theme.colors.muted} opacity={0.8} />
      </g>
    );
  }
  const p = e.paragraphs[0];
  const r = p?.runs[0];
  if (!r) return null;
  const fill = resolveColor(theme, r.color);
  if (p.bullet) {
    return (
      <g>
        {[0, 1, 2, 3].map((k) => {
          const y = f.y + 30 + k * 120;
          const w = (f.w - 100) * [0.85, 0.7, 0.55, 0.8][k];
          return (
            <g key={k}>
              {p.bullet === "number" ? (
                <text x={f.x} y={y + 50} fontSize={80} fontWeight={700} fill={theme.colors.accent}>{k + 1}</text>
              ) : (
                <circle cx={f.x + 22} cy={y + 25} r={20} fill={fill} />
              )}
              <rect x={f.x + 90} y={y} width={w} height={50} rx={14} fill={fill} opacity={0.7} />
            </g>
          );
        })}
      </g>
    );
  }
  if (r.size >= 200) {
    return <text x={f.x + f.w / 2} y={f.y + f.h * 0.85} textAnchor="middle" fontSize={f.h * 0.8} fontWeight={800} fill={theme.colors.accent}>42%</text>;
  }
  if (r.italic) {
    return (
      <g>
        <text x={f.x - 40} y={f.y + 200} fontSize={300} fontFamily="Georgia, serif" fill={theme.colors.accent}>“</text>
        {[0, 1, 2].map((k) => <rect key={k} x={f.x + 160} y={f.y + 120 + k * 120} width={(f.w - 160) * (k < 2 ? 0.9 : 0.5)} height={70} rx={16} fill={fill} />)}
      </g>
    );
  }
  if (e.placeholder === "body" && f.h > 300) {
    return <g>{[0, 1, 2].map((k) => <rect key={k} x={f.x} y={f.y + 30 + k * 110} width={f.w * [0.9, 0.8, 0.5][k]} height={50} rx={14} fill={fill} opacity={0.7} />)}</g>;
  }
  // One bar, its height from the text size, placed by align and valign.
  const h = Math.max(40, r.size * 0.55);
  const w = Math.min(f.w * (e.placeholder === "title" ? 0.6 : 0.35), e.placeholder === "title" ? 1000 : 600);
  const x = p.align === "center" ? f.x + (f.w - w) / 2 : f.x;
  const y = e.valign === "bottom" ? f.y + f.h - h - 20 : e.valign === "middle" ? f.y + (f.h - h) / 2 : f.y + 20;
  return <rect x={x} y={y} width={w} height={h} rx={14} fill={fill} />;
}

/**
 * The layouts as a grid of thumbnails, for a Radix dropdown's content. Radix
 * moves focus with ↑/↓; ←/→ move one tile too, so the grid reads naturally.
 */
export function LayoutGrid({ theme, current, onPick, title }: { theme: Theme; current?: Layout; onPick: (l: Layout) => void; title: string }) {
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    const items = [...e.currentTarget.querySelectorAll<HTMLElement>("[role=menuitem]")];
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (i < 0) return;
    e.preventDefault();
    e.stopPropagation();
    items[Math.max(0, Math.min(items.length - 1, i + (e.key === "ArrowLeft" ? -1 : 1)))].focus();
  };
  return (
    <div className="w-[min(520px,90vw)] p-1.5" onKeyDown={onKey}>
      <div className="flex items-baseline justify-between px-1 pb-2">
        <span className="text-[13px] font-semibold text-fg">{title}</span>
        <span className="text-[11.5px] text-fg-tertiary">Theme: {theme.name}</span>
      </div>
      <div role="group" aria-label={title} className="grid grid-cols-4 gap-x-3 gap-y-2.5 max-sm:grid-cols-3">
        {LAYOUTS.map((l) => (
          <DropdownMenu.Item
            key={l.id}
            data-layout={l.id}
            aria-label={l.label}
            aria-current={l.id === current ? "true" : undefined}
            onSelect={() => onPick(l.id)}
            className="group flex cursor-pointer select-none flex-col gap-1 rounded-md p-0.5 outline-none data-[highlighted]:bg-hover"
          >
            <span className={cx("relative block overflow-hidden rounded-[3px] border", l.id === current ? "border-accent ring-2 ring-accent/60" : "border-border-strong group-data-[highlighted]:border-fg-tertiary")}>
              <LayoutThumb layout={l.id} theme={theme} />
              {NEW_LAYOUTS.has(l.id) && <span aria-hidden className="absolute right-1 top-1 rounded-full bg-accent px-1.5 text-[8.5px] font-bold leading-[13px] text-[#0E0F14]">NEW</span>}
            </span>
            <span className={cx("truncate text-[11.5px]", l.id === current ? "font-semibold text-fg" : "text-fg-secondary")}>{l.label}</span>
          </DropdownMenu.Item>
        ))}
      </div>
    </div>
  );
}
