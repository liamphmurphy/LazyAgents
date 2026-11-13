"use client";

/**
 * The fill, outline, line, image and background menus (LAZYAGENTS-118
 * mockup): solid or gradient fills, opacity sliders and dash styles. They act
 * on every selected element; what they show comes from the first one, and a
 * value the selection doesn't share shows as mixed on the chip.
 */
import { DropdownMenu } from "radix-ui";
import { useState } from "react";
import { cx } from "@/lib/cx";
import { floatPanel } from "@/components/domain/pickers";
import { DASHES, isGradient, paintCss, resolveColor, resolvePaint, type ColorRef, type Dash, type Fill, type Gradient, type ImageElement, type LineElement, type ShapeElement, type Slide, type Theme } from "@/lib/slides/schema";
import { ColorItems, Menu, colorName, item, menuSep, ownKeys } from "./menu";

/** Style fields a menu sets on the selected elements; undefined clears one. */
export type StylePatch = Partial<{ fill: Fill; fillOpacity: number; strokeOpacity: number; dash: Dash; opacity: number }>;

export type StyleActions = {
  /** key coalesces repeated changes (dragging the angle) into one undo step. */
  setFill: (f: Fill | undefined, key?: string) => void;
  setStroke: (c: ColorRef | undefined) => void;
  setStyle: (patch: StylePatch, key?: string) => void;
  setBackground: (f: Fill | undefined, key?: string) => void;
  /** Custom colours used in this deck, newest first. */
  recentColors: string[];
  rememberColor: (c: string) => void;
};

const heading = "px-2.5 pb-1 pt-1.5 text-13 text-fg-2";
const mixedSwatch = "repeating-linear-gradient(45deg, #c7c7cc 0 3px, transparent 3px 6px)";
const same = <T,>(xs: T[]) => xs.every((x) => JSON.stringify(x) === JSON.stringify(xs[0]));

function Opacity({ value, onChange, label }: { value: number | undefined; onChange: (v: number | undefined) => void; label: string }) {
  const pct = Math.round((value ?? 1) * 100);
  return (
    <div className="px-2.5 py-2" onKeyDown={ownKeys}>
      <div className="flex items-center justify-between text-13 text-fg-2">
        <span>Opacity</span>
        <span className="font-mono text-fg">{pct}%</span>
      </div>
      <input
        type="range"
        min={0}
        max={100}
        step={5}
        value={pct}
        aria-label={label}
        aria-valuetext={`${pct}%`}
        className="mt-1.5 w-full accent-accent"
        onChange={(e) => {
          const v = Number(e.target.value);
          onChange(v >= 100 ? undefined : v / 100);
        }}
      />
    </div>
  );
}

function Tabs({ tab, setTab }: { tab: "solid" | "gradient"; setTab: (t: "solid" | "gradient") => void }) {
  return (
    <div role="tablist" className="m-1 mb-1.5 grid grid-cols-2 gap-0.5 rounded-md bg-hover p-0.5" onKeyDown={ownKeys}>
      {(["solid", "gradient"] as const).map((t) => (
        <button
          key={t}
          type="button"
          role="tab"
          aria-selected={tab === t}
          className={cx("rounded-[5px] px-2 py-1 text-13 outline-none focus-visible:ring-2 focus-visible:ring-accent", tab === t ? "border border-border-strong bg-surface font-semibold text-fg" : "text-fg-2")}
          onClick={() => setTab(t)}
        >
          {t === "solid" ? "Solid" : "Gradient"}
        </button>
      ))}
    </div>
  );
}

function StopPicker({ theme, label, value, onPick, a }: { theme: Theme; label: string; value: ColorRef; onPick: (c: ColorRef) => void; a: StyleActions }) {
  return (
    <>
      <div className={heading}>{label}</div>
      <DropdownMenu.Sub>
        <DropdownMenu.SubTrigger className={cx(item, "mx-1 border border-border-strong")} aria-label={`${label}: ${colorName(value)}`}>
          <span className="size-4 rounded-sm border border-border-strong" style={{ background: resolveColor(theme, value) }} />
          {colorName(value)}
          <span className="ml-auto text-fg-2">▸</span>
        </DropdownMenu.SubTrigger>
        <DropdownMenu.Portal>
          <DropdownMenu.SubContent sideOffset={6} className={cx(floatPanel, "z-50 max-h-[70vh] min-w-40 overflow-y-auto")}>
            <ColorItems label={label} theme={theme} current={value} onPick={(c) => c && onPick(c)} recent={a.recentColors} remember={a.rememberColor} />
          </DropdownMenu.SubContent>
        </DropdownMenu.Portal>
      </DropdownMenu.Sub>
    </>
  );
}

const QUICK = [
  { angle: 0, glyph: "→", label: "Left to right" },
  { angle: 45, glyph: "↘", label: "Diagonal" },
  { angle: 90, glyph: "↓", label: "Top to bottom" },
];

function GradientEditor({ theme, g, onChange, a }: { theme: Theme; g: Gradient; onChange: (g: Gradient, key?: string) => void; a: StyleActions }) {
  const angle = (deg: number) => onChange({ ...g, angle: ((Math.round(deg) % 360) + 360) % 360 }, "gradient-angle");
  const rad = (g.angle * Math.PI) / 180;
  const fromPointer = (e: React.PointerEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    angle(Math.round(((Math.atan2(e.clientY - r.top - r.height / 2, e.clientX - r.left - r.width / 2) * 180) / Math.PI) / 5) * 5);
  };
  return (
    <div className="w-60">
      <div aria-hidden className="mx-2.5 my-1.5 h-12 rounded-md border border-border-strong" style={{ background: paintCss(resolvePaint(theme, g)) }} />
      <StopPicker theme={theme} label="Start colour" value={g.stops[0]} onPick={(c) => onChange({ ...g, stops: [c, g.stops[1]] })} a={a} />
      <StopPicker theme={theme} label="End colour" value={g.stops[1]} onPick={(c) => onChange({ ...g, stops: [g.stops[0], c] })} a={a} />
      <div className={heading}>Angle</div>
      <div className="flex items-center gap-1.5 px-2.5 pb-1.5" onKeyDown={ownKeys}>
        <svg
          width={40}
          height={40}
          viewBox="-20 -20 40 40"
          className="shrink-0 cursor-pointer touch-none"
          aria-hidden
          onPointerDown={(e) => {
            e.currentTarget.setPointerCapture(e.pointerId);
            fromPointer(e);
          }}
          onPointerMove={(e) => e.buttons && fromPointer(e)}
        >
          <circle r={19} className="fill-surface stroke-border-strong" />
          <line x1={0} y1={0} x2={Math.cos(rad) * 14} y2={Math.sin(rad) * 14} className="stroke-accent" strokeWidth={2} />
          <circle cx={Math.cos(rad) * 14} cy={Math.sin(rad) * 14} r={3} className="fill-accent" />
        </svg>
        <input
          type="number"
          min={0}
          max={359}
          step={1}
          value={g.angle}
          aria-label="Gradient angle in degrees"
          className="h-7 w-14 rounded-md border border-border-strong bg-surface px-1.5 font-mono text-13 text-fg outline-none focus-visible:ring-2 focus-visible:ring-accent"
          onChange={(e) => e.target.value !== "" && Number.isFinite(Number(e.target.value)) && angle(Number(e.target.value))}
        />
        <span className="text-13 text-fg-2">°</span>
        {QUICK.map((q) => (
          <button key={q.angle} type="button" title={q.label} aria-label={q.label} aria-pressed={g.angle === q.angle} className={cx("h-7 w-6 rounded-md border text-12", g.angle === q.angle ? "border-accent bg-selected" : "border-border-strong")} onClick={() => onChange({ ...g, angle: q.angle })}>
            {q.glyph}
          </button>
        ))}
        <button type="button" title="Swap colours" aria-label="Swap colours" className="h-7 w-6 rounded-md border border-border-strong text-12" onClick={() => onChange({ ...g, stops: [g.stops[1], g.stops[0]] })}>
          ⇄
        </button>
      </div>
    </div>
  );
}

/** A gradient to start from: the current colour fading into another theme colour. */
const seed = (from: ColorRef | undefined, fallback: ColorRef, to: ColorRef): Gradient => {
  const start = from ?? fallback;
  return { type: "linear", angle: 90, stops: [start, start !== to ? to : to === "accent2" ? "accent" : "accent2"] };
};

/** Solid and Gradient tabs over a fill; the Gradient tab seeds a gradient from the current colour. */
function FillTabs({ theme, label, value, none, fallback, to, onFill, onDone, a }: { theme: Theme; label: string; value: Fill | undefined; none: string; fallback: ColorRef; to: ColorRef; onFill: (f: Fill | undefined, key?: string) => void; onDone: () => void; a: StyleActions }) {
  const [tab, setTab] = useState<"solid" | "gradient">(isGradient(value) ? "gradient" : "solid");
  const pick = (t: "solid" | "gradient") => {
    setTab(t);
    if (t === "gradient" && !isGradient(value)) onFill(seed(value, fallback, to));
  };
  return (
    <>
      <Tabs tab={tab} setTab={pick} />
      {tab === "solid" ? (
        <ColorItems label={label} theme={theme} onPick={(c) => onFill(c)} none={none} current={value === undefined ? null : isGradient(value) ? undefined : value} recent={a.recentColors} remember={a.rememberColor} onDone={onDone} />
      ) : (
        isGradient(value) && <GradientEditor theme={theme} g={value} onChange={onFill} a={a} />
      )}
    </>
  );
}

function DashLine({ pattern, width = 96 }: { pattern: number[]; width?: number }) {
  return (
    <svg width={width} height={8} aria-hidden className="shrink-0">
      <line x1={0} y1={4} x2={width} y2={4} stroke="currentColor" strokeWidth={2} strokeDasharray={pattern.length ? pattern.map((n) => n * 2).join(" ") : undefined} />
    </svg>
  );
}

function DashItems({ value, onPick }: { value: Dash | undefined; onPick: (d: Dash | undefined) => void }) {
  return (
    <>
      {DASHES.map((d) => {
        const current = (value ?? "solid") === d.id;
        return (
          <DropdownMenu.Item key={d.id} className={cx(item, current && "bg-hover font-semibold")} onSelect={() => onPick(d.id === "solid" ? undefined : d.id)}>
            <DashLine pattern={d.pattern} />
            {d.label}
            {current && <span aria-hidden className="ml-auto pl-3">✓</span>}
          </DropdownMenu.Item>
        );
      })}
    </>
  );
}

export function FillMenu({ theme, shapes, a }: { theme: Theme; shapes: ShapeElement[]; a: StyleActions }) {
  const [open, setOpen] = useState(false);
  const el = shapes[0];
  const mixed = !same(shapes.map((e) => [e.fill, e.fillOpacity]));
  return (
    <Menu
      label="Fill"
      open={open}
      onOpenChange={setOpen}
      trigger={
        <>
          <span className="size-3 rounded-sm border border-border-strong" title={mixed ? "Mixed fills" : undefined} style={{ background: mixed ? mixedSwatch : el.fill ? paintCss(resolvePaint(theme, el.fill)) : "transparent", opacity: mixed ? undefined : el.fillOpacity }} />
          Fill ▾
        </>
      }
    >
      <FillTabs theme={theme} label="Fill" value={el.fill} none="No fill" fallback="accent" to="accent2" onFill={a.setFill} onDone={() => setOpen(false)} a={a} />
      {el.fill !== undefined && (
        <>
          {menuSep}
          <Opacity label="Fill opacity" value={el.fillOpacity} onChange={(v) => a.setStyle({ fillOpacity: v }, "fill-opacity")} />
        </>
      )}
    </Menu>
  );
}

export function OutlineMenu({ theme, shapes, a }: { theme: Theme; shapes: ShapeElement[]; a: StyleActions }) {
  const [open, setOpen] = useState(false);
  const el = shapes[0];
  const pattern = DASHES.find((d) => d.id === el.dash)?.pattern ?? [];
  return (
    <Menu label="Outline" open={open} onOpenChange={setOpen} trigger={<>{el.stroke && <DashLine pattern={pattern.map((n) => n * 0.75)} width={18} />}Outline ▾</>}>
      <ColorItems label="Outline" theme={theme} onPick={a.setStroke} none="No outline" current={el.stroke ?? null} recent={a.recentColors} remember={a.rememberColor} onDone={() => setOpen(false)} />
      {el.stroke && (
        <>
          {menuSep}
          <div className={heading}>Dash</div>
          <DashItems value={el.dash} onPick={(d) => a.setStyle({ dash: d })} />
          {menuSep}
          <Opacity label="Outline opacity" value={el.strokeOpacity} onChange={(v) => a.setStyle({ strokeOpacity: v }, "stroke-opacity")} />
        </>
      )}
    </Menu>
  );
}

export function LineMenu({ theme, lines, a }: { theme: Theme; lines: LineElement[]; a: StyleActions }) {
  const [open, setOpen] = useState(false);
  const el = lines[0];
  const dash = DASHES.find((d) => d.id === (el.dash ?? "solid"))!;
  return (
    <Menu label="Line colour" open={open} onOpenChange={setOpen} trigger={<>Colour ▾</>}>
      <ColorItems label="Line colour" theme={theme} onPick={(c) => c && a.setStroke(c)} current={el.stroke} recent={a.recentColors} remember={a.rememberColor} onDone={() => setOpen(false)} />
      {menuSep}
      <div className={heading}>Dash</div>
      <DropdownMenu.Sub>
        <DropdownMenu.SubTrigger className={cx(item, "mx-1 border border-border-strong")} aria-label={`Dash: ${dash.label}`}>
          <DashLine pattern={dash.pattern} />
          {dash.label}
          <span className="ml-auto text-fg-2">▸</span>
        </DropdownMenu.SubTrigger>
        <DropdownMenu.Portal>
          <DropdownMenu.SubContent sideOffset={6} className={cx(floatPanel, "z-50 min-w-48")}>
            <DashItems value={el.dash} onPick={(d) => a.setStyle({ dash: d })} />
          </DropdownMenu.SubContent>
        </DropdownMenu.Portal>
      </DropdownMenu.Sub>
      <Opacity label="Line opacity" value={el.strokeOpacity} onChange={(v) => a.setStyle({ strokeOpacity: v }, "stroke-opacity")} />
    </Menu>
  );
}

export function ImageOpacityMenu({ images, a }: { images: ImageElement[]; a: StyleActions }) {
  return (
    <Menu label="Image opacity" trigger={<>◐ Opacity ▾</>} className="w-56">
      <Opacity label="Image opacity" value={images[0].opacity} onChange={(v) => a.setStyle({ opacity: v }, "image-opacity")} />
    </Menu>
  );
}

export function BackgroundMenu({ theme, slide, a }: { theme: Theme; slide: Slide; a: StyleActions }) {
  const [open, setOpen] = useState(false);
  return (
    <Menu label="Slide background" open={open} onOpenChange={setOpen} trigger={<>Background ▾</>}>
      <FillTabs theme={theme} label="Slide background" value={slide.background?.color} none="Theme background" fallback="background" to="accent" onFill={a.setBackground} onDone={() => setOpen(false)} a={a} />
    </Menu>
  );
}
