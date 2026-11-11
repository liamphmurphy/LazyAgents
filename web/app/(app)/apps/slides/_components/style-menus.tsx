"use client";

/**
 * The fill, outline, line, image and background menus (LAZYAGENTS-118
 * mockup): solid or gradient fills, opacity sliders and dash styles.
 */
import { DropdownMenu } from "radix-ui";
import { useState } from "react";
import { cx } from "@/lib/cx";
import { floatPanel } from "@/components/domain/pickers";
import { DASHES, isGradient, paintCss, resolveColor, resolvePaint, type ColorRef, type Dash, type Fill, type Gradient, type ImageElement, type LineElement, type ShapeElement, type Slide, type Theme } from "@/lib/slides/schema";
import { Menu, Swatches, colorName, item, ownKeys } from "./menu";

/** Style fields a menu sets on the selected element; undefined clears one. */
export type StylePatch = Partial<{ fill: Fill; fillOpacity: number; strokeOpacity: number; dash: Dash; opacity: number }>;

export type StyleActions = {
  setFill: (f: Fill | undefined, key?: string) => void;
  setStroke: (c: ColorRef | undefined) => void;
  setStyle: (patch: StylePatch, key?: string) => void;
  setBackground: (f: Fill | undefined, key?: string) => void;
};

const sep = <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />;
const heading = "px-2.5 pb-1 pt-1.5 text-[12.5px] text-fg-2";

function Opacity({ value, onChange, label }: { value: number | undefined; onChange: (v: number | undefined) => void; label: string }) {
  const pct = Math.round((value ?? 1) * 100);
  return (
    <div className="px-2.5 py-2" onKeyDown={ownKeys}>
      <div className="flex items-center justify-between text-[13px] text-fg-2">
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
          className={cx("rounded-[5px] px-2 py-1 text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-accent", tab === t ? "border border-border-strong bg-surface font-semibold text-fg" : "text-fg-2")}
          onClick={() => setTab(t)}
        >
          {t === "solid" ? "Solid" : "Gradient"}
        </button>
      ))}
    </div>
  );
}

function StopPicker({ theme, label, value, onPick }: { theme: Theme; label: string; value: ColorRef; onPick: (c: ColorRef) => void }) {
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
          <DropdownMenu.SubContent sideOffset={6} className={cx(floatPanel, "z-50 min-w-40")}>
            <Swatches theme={theme} current={value} onPick={(c) => c && onPick(c)} />
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

function GradientEditor({ theme, g, onChange }: { theme: Theme; g: Gradient; onChange: (g: Gradient, key?: string) => void }) {
  const angle = (deg: number) => onChange({ ...g, angle: ((Math.round(deg) % 360) + 360) % 360 }, "gradient-angle");
  const rad = (g.angle * Math.PI) / 180;
  const fromPointer = (e: React.PointerEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    angle(Math.round(((Math.atan2(e.clientY - r.top - r.height / 2, e.clientX - r.left - r.width / 2) * 180) / Math.PI) / 5) * 5);
  };
  return (
    <div className="w-60">
      <div aria-hidden className="mx-2.5 my-1.5 h-12 rounded-md border border-border-strong" style={{ background: paintCss(resolvePaint(theme, g)) }} />
      <StopPicker theme={theme} label="Start colour" value={g.stops[0]} onPick={(c) => onChange({ ...g, stops: [c, g.stops[1]] })} />
      <StopPicker theme={theme} label="End colour" value={g.stops[1]} onPick={(c) => onChange({ ...g, stops: [g.stops[0], c] })} />
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
          className="h-7 w-14 rounded-md border border-border-strong bg-surface px-1.5 font-mono text-[12.5px] text-fg outline-none focus-visible:ring-2 focus-visible:ring-accent"
          onChange={(e) => e.target.value !== "" && Number.isFinite(Number(e.target.value)) && angle(Number(e.target.value))}
        />
        <span className="text-[12.5px] text-fg-2">°</span>
        {QUICK.map((q) => (
          <button key={q.angle} type="button" title={q.label} aria-label={q.label} aria-pressed={g.angle === q.angle} className={cx("h-7 w-6 rounded-md border text-[12px]", g.angle === q.angle ? "border-accent bg-selected" : "border-border-strong")} onClick={() => onChange({ ...g, angle: q.angle })}>
            {q.glyph}
          </button>
        ))}
        <button type="button" title="Swap colours" aria-label="Swap colours" className="h-7 w-6 rounded-md border border-border-strong text-[12px]" onClick={() => onChange({ ...g, stops: [g.stops[1], g.stops[0]] })}>
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

function FillPanel({ theme, el, a }: { theme: Theme; el: ShapeElement; a: StyleActions }) {
  const [tab, setTab] = useState<"solid" | "gradient">(isGradient(el.fill) ? "gradient" : "solid");
  const pick = (t: "solid" | "gradient") => {
    setTab(t);
    if (t === "gradient" && !isGradient(el.fill)) a.setFill(seed(el.fill, "accent", "accent2"));
  };
  return (
    <>
      <Tabs tab={tab} setTab={pick} />
      {tab === "solid" ? <Swatches theme={theme} onPick={(c) => a.setFill(c)} none="No fill" current={el.fill === undefined ? null : isGradient(el.fill) ? undefined : el.fill} /> : isGradient(el.fill) && <GradientEditor theme={theme} g={el.fill} onChange={(g, key) => a.setFill(g, key)} />}
      {el.fill !== undefined && (
        <>
          {sep}
          <Opacity label="Fill opacity" value={el.fillOpacity} onChange={(v) => a.setStyle({ fillOpacity: v }, "fill-opacity")} />
        </>
      )}
    </>
  );
}

function DashLine({ pattern }: { pattern: number[] }) {
  return (
    <svg width={96} height={8} aria-hidden className="shrink-0 text-fg">
      <line x1={0} y1={4} x2={96} y2={4} stroke="currentColor" strokeWidth={2} strokeDasharray={pattern.length ? pattern.map((n) => n * 2).join(" ") : undefined} />
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
            {current && " ✓"}
          </DropdownMenu.Item>
        );
      })}
    </>
  );
}

export function FillMenu({ theme, el, a }: { theme: Theme; el: ShapeElement; a: StyleActions }) {
  return (
    <Menu label="Fill" trigger={<><span className="size-3 rounded-sm border border-border-strong" style={{ background: el.fill ? paintCss(resolvePaint(theme, el.fill)) : "transparent", opacity: el.fillOpacity }} />Fill ▾</>}>
      <FillPanel theme={theme} el={el} a={a} />
    </Menu>
  );
}

export function OutlineMenu({ theme, el, a }: { theme: Theme; el: ShapeElement; a: StyleActions }) {
  const pattern = DASHES.find((d) => d.id === el.dash)?.pattern ?? [];
  return (
    <Menu
      label="Outline"
      trigger={
        <>
          {el.stroke && (
            <svg width={18} height={4} aria-hidden>
              <line x1={0} y1={2} x2={18} y2={2} stroke="currentColor" strokeWidth={2} strokeDasharray={pattern.length ? pattern.map((n) => n * 1.5).join(" ") : undefined} />
            </svg>
          )}
          Outline ▾
        </>
      }
    >
      <Swatches theme={theme} onPick={a.setStroke} none="No outline" current={el.stroke ?? null} />
      {el.stroke && (
        <>
          {sep}
          <div className={heading}>Dash</div>
          <DashItems value={el.dash} onPick={(d) => a.setStyle({ dash: d })} />
          {sep}
          <Opacity label="Outline opacity" value={el.strokeOpacity} onChange={(v) => a.setStyle({ strokeOpacity: v }, "stroke-opacity")} />
        </>
      )}
    </Menu>
  );
}

export function LineMenu({ theme, el, a }: { theme: Theme; el: LineElement; a: StyleActions }) {
  const dash = DASHES.find((d) => d.id === (el.dash ?? "solid"))!;
  return (
    <Menu label="Line colour" trigger={<>Colour ▾</>}>
      <Swatches theme={theme} onPick={(c) => c && a.setStroke(c)} current={el.stroke} />
      {sep}
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

export function ImageOpacityMenu({ el, a }: { el: ImageElement; a: StyleActions }) {
  return (
    <Menu label="Image opacity" trigger={<>◐ Opacity ▾</>} className="w-56">
      <Opacity label="Image opacity" value={el.opacity} onChange={(v) => a.setStyle({ opacity: v }, "image-opacity")} />
    </Menu>
  );
}

function BackgroundPanel({ theme, slide, a }: { theme: Theme; slide: Slide; a: StyleActions }) {
  const bg = slide.background?.color;
  const [tab, setTab] = useState<"solid" | "gradient">(isGradient(bg) ? "gradient" : "solid");
  const pick = (t: "solid" | "gradient") => {
    setTab(t);
    if (t === "gradient" && !isGradient(bg)) a.setBackground(seed(bg, "background", "accent"));
  };
  return (
    <>
      <Tabs tab={tab} setTab={pick} />
      {tab === "solid" ? <Swatches theme={theme} onPick={(c) => a.setBackground(c)} none="Theme background" current={bg === undefined ? null : isGradient(bg) ? undefined : bg} /> : isGradient(bg) && <GradientEditor theme={theme} g={bg} onChange={(g, key) => a.setBackground(g, key)} />}
    </>
  );
}

export function BackgroundMenu({ theme, slide, a }: { theme: Theme; slide: Slide; a: StyleActions }) {
  return (
    <Menu label="Slide background" trigger={<>Background ▾</>}>
      <BackgroundPanel theme={theme} slide={slide} a={a} />
    </Menu>
  );
}
