"use client";

import { useEffect, useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { PRESETS, find, inAutoLayout, mapNodes, presetLabel, resizeIn, resizeNode } from "@/lib/designs/edit";
import type { Layout } from "@/lib/designs/layout";
import { ComponentSection, SendToTicket } from "./library-panels";
import { SHAPE_PRESETS, type AutoLayout, type Constraints, type Design, type DesignNode, type Paint, type Paragraph, type Run } from "@/lib/designs/schema";

type Props = {
  design: Design;
  selected: string[];
  lay: Layout;
  onEdit: (fn: (d: Design) => Design, key?: string) => void;
  readOnly: boolean;
  onGroup: () => void;
  onRemove: () => void;
  onSelect: (ids: string[]) => void;
};

const section = "border-b border-border-subtle px-4 py-3";
const label = "mb-2 text-[11px] font-semibold uppercase tracking-[0.08em] text-fg-tertiary";
const field = "flex h-[28px] min-w-0 items-center gap-1.5 rounded-md border border-border bg-surface-sunken px-2 text-[12.5px] text-fg focus-within:border-accent";
const seg = (on: boolean) => cx("h-[26px] flex-1 rounded-md px-1.5 text-[12px]", on ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover");

/** A number field that commits as you type (one undo step per field while typing). */
function Num({ label: l, value, onChange, min, max, step = 1, disabled, suffix }: { label: string; value: number | undefined; onChange: (v: number) => void; min?: number; max?: number; step?: number; disabled?: boolean; suffix?: string }) {
  const [text, setText] = useState(value === undefined ? "" : String(Math.round(value * 100) / 100));
  const [focused, setFocused] = useState(false);
  useEffect(() => {
    // Follow the design (a drag on the canvas, undo) unless the person is typing here.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (!focused) setText(value === undefined ? "" : String(Math.round(value * 100) / 100));
  }, [value, focused]);
  return (
    <label className={cx(field, disabled && "opacity-50")}>
      <span className="min-w-3.5 flex-none text-[11px] text-fg-tertiary">{l}</span>
      <input
        aria-label={l.length > 2 ? l : undefined}
        inputMode="decimal"
        disabled={disabled}
        className="w-full min-w-0 bg-transparent outline-none"
        value={text}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onChange={(e) => {
          setText(e.target.value);
          const v = Number(e.target.value);
          if (e.target.value.trim() === "" || !Number.isFinite(v)) return;
          onChange(Math.min(max ?? Infinity, Math.max(min ?? -Infinity, v)));
        }}
        onKeyDown={(e) => {
          if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
          e.preventDefault();
          const v = (Number(text) || 0) + (e.key === "ArrowUp" ? 1 : -1) * step * (e.shiftKey ? 10 : 1);
          const c = Math.min(max ?? Infinity, Math.max(min ?? -Infinity, v));
          setText(String(c));
          onChange(c);
        }}
      />
      {suffix && <span className="text-[11px] text-fg-tertiary">{suffix}</span>}
    </label>
  );
}

function Color({ name, value, onChange, disabled, resolved }: { name: string; value: Paint | undefined; onChange: (v: string | undefined) => void; disabled?: boolean; resolved?: string }) {
  const token = typeof value === "string" && value.startsWith("$") ? value.slice(1) : undefined;
  const hex = typeof value === "string" && !token ? value.slice(0, 7) : (resolved?.slice(0, 7) ?? "#000000");
  const [text, setText] = useState(typeof value === "string" ? value : "");
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setText(typeof value === "string" ? value : "");
  }, [value]);
  return (
    <div className={cx(field, "pr-1")}>
      <input type="color" aria-label={`${name} colour`} disabled={disabled} value={/^#[0-9A-Fa-f]{6}$/.test(hex) ? hex : "#000000"} onChange={(e) => onChange(e.target.value.toUpperCase())} className="size-4 flex-none cursor-pointer rounded border-0 bg-transparent p-0" />
      {typeof value === "object" ? (
        <span className="flex-1 text-fg-secondary">Gradient</span>
      ) : (
        <input
          aria-label={name}
          disabled={disabled}
          className="w-full min-w-0 bg-transparent font-mono text-[12px] outline-none"
          value={token ? `${token}` : text}
          placeholder="None"
          onChange={(e) => {
            const v = e.target.value.trim();
            setText(v);
            if (!v) onChange(undefined);
            else if (/^#?[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/.test(v)) onChange(`#${v.replace("#", "").toUpperCase()}`);
          }}
        />
      )}
      {token && <span className="text-[11px] text-accent">token</span>}
      {value !== undefined && !disabled && (
        <button type="button" aria-label={`Remove ${name.toLowerCase()}`} className="grid size-5 place-items-center rounded text-fg-tertiary hover:bg-hover hover:text-fg" onClick={() => onChange(undefined)}>
          −
        </button>
      )}
    </div>
  );
}

function Section({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <section className={section} aria-label={title}>
      <div className="flex items-center justify-between">
        <h3 className={label}>{title}</h3>
        {action}
      </div>
      {children}
    </section>
  );
}

const TYPE_LABEL: Record<string, string> = { frame: "Frame", component: "Component", instance: "Instance", group: "Group", section: "Section", shape: "Shape", path: "Vector", text: "Text", image: "Image", sticky: "Sticky", connector: "Connector" };
const SHAPE_LABEL = (s: string) => ({ rect: "Rectangle", roundRect: "Rounded rectangle", ellipse: "Ellipse" })[s] ?? s.replace(/([A-Z])/g, " $1").replace(/^./, (c) => c.toUpperCase());

/** Every run of every paragraph through fn. */
const mapRuns = (ps: Paragraph[], fn: (r: Run) => Run) => ps.map((p) => ({ ...p, runs: p.runs.map(fn) }));

/** The inspector's Design tab (docs/design/designs-editor.svg, right column). */
export function Inspector({ design, selected, lay, onEdit, readOnly, onGroup, onRemove, onSelect }: Props) {
  const found = selected.map((id) => find(design, id)).filter((f) => !!f);
  if (!found.length) {
    return <p className="px-4 py-6 text-[12.5px] text-fg-tertiary">Select a layer to edit it. Draw a frame (F), a rectangle (R), an ellipse (O), a line (L), a path with the pen (P) or text (T).</p>;
  }
  if (found.length > 1) {
    return (
      <div className={section}>
        <p className="text-[13px] font-semibold text-fg">{found.length} layers</p>
        <div className="mt-3 flex gap-2">
          <button type="button" disabled={readOnly} className="h-[28px] flex-1 rounded-md border border-border text-[12.5px] text-fg hover:bg-hover disabled:opacity-50" onClick={onGroup}>Group ⌘G</button>
          <button type="button" disabled={readOnly} className="h-[28px] flex-1 rounded-md border border-border text-[12.5px] text-pill-blocked-fg hover:bg-hover disabled:opacity-50" onClick={onRemove}>Delete</button>
        </div>
      </div>
    );
  }
  const { node: n, parent } = found[0];
  const id = n.id;
  const key = (k: string) => `${id}:${k}`;
  const set = (fn: (x: DesignNode) => DesignNode, k: string) => onEdit((d) => mapNodes(d, new Set([id]), fn), key(k));
  const patch = (fields: Record<string, unknown>, k: string) =>
    set((x) => {
      const out: Record<string, unknown> = { ...x };
      for (const [f, v] of Object.entries(fields)) {
        if (v === undefined) delete out[f];
        else out[f] = v;
      }
      return out as DesignNode;
    }, k);
  if (n.type === "connector") return <p className="px-4 py-6 text-[12.5px] text-fg-tertiary">Connectors are edited with the diagram tools.</p>;

  const box = lay.widthOf(n);
  const al = "auto_layout" in n ? n.auto_layout : undefined;
  const autoLaid = inAutoLayout(parent);
  const container = n.type === "frame" || n.type === "component";
  const sizing = { w: n.sizing?.w ?? "fixed", h: n.sizing?.h ?? "fixed" };
  const canHug = n.type === "text" || n.type === "instance" || (container && !!al);
  const resize = (b: { x?: number; y?: number; w?: number; h?: number }, k: string) => onEdit((d) => resizeIn(d, id, (x) => (x.type === "connector" ? { x: 0, y: 0, w: 0, h: 0 } : { x: b.x ?? x.x, y: b.y ?? x.y, w: b.w ?? x.w, h: b.h ?? x.h })), key(k));
  const setAl = (p: Partial<AutoLayout>) => patch({ auto_layout: al ? { ...al, ...p } : undefined }, "al");
  const setSizing = (axis: "w" | "h", v: string) => {
    const next = { ...n.sizing, [axis]: v === "fixed" ? undefined : v };
    if (!next.w) delete next.w;
    if (!next.h) delete next.h;
    patch({ sizing: Object.keys(next).length ? next : undefined }, "sizing");
  };
  const constraints = (n as { constraints?: Constraints }).constraints ?? {};
  const setConstraint = (axis: "h" | "v", v: string) => {
    const next = { ...constraints, [axis]: v === "left" || v === "top" ? undefined : v };
    if (!next.h) delete next.h;
    if (!next.v) delete next.v;
    patch({ constraints: Object.keys(next).length ? next : undefined }, "constraints");
  };
  const styled = n.type === "frame" || n.type === "component" || n.type === "shape" || n.type === "path";
  const firstRun = n.type === "text" ? n.paragraphs[0]?.runs[0] : n.type === "shape" ? n.text?.paragraphs[0]?.runs[0] : undefined;
  const paragraphs = n.type === "text" ? n.paragraphs : n.type === "shape" ? n.text?.paragraphs : undefined;
  const setText = (fn: (ps: Paragraph[]) => Paragraph[], k: string) =>
    set((x) => (x.type === "text" ? { ...x, paragraphs: fn(x.paragraphs) } : x.type === "shape" && x.text ? { ...x, text: { ...x.text, paragraphs: fn(x.text.paragraphs) } } : x), k);
  const mode = design.mode;

  return (
    <div className="pb-6" data-testid="design-inspector">
      <div className={section}>
        <div className="flex items-baseline justify-between gap-2">
          <p className="truncate text-[13px] font-semibold text-fg">{n.name ?? TYPE_LABEL[n.type]}</p>
          <p className="flex-none text-[11.5px] text-fg-tertiary">
            {TYPE_LABEL[n.type]}
            {"preset" in n && presetLabel(n.preset) ? ` · ${presetLabel(n.preset)}` : ""}
          </p>
        </div>
        <div className="mt-3 grid grid-cols-2 gap-2">
          <Num label="X" value={n.x} disabled={readOnly || autoLaid} onChange={(v) => patch({ x: v }, "x")} />
          <Num label="Y" value={n.y} disabled={readOnly || autoLaid} onChange={(v) => patch({ y: v }, "y")} />
          <Num label="W" value={box} min={1} disabled={readOnly || sizing.w !== "fixed"} onChange={(v) => resize({ w: v }, "w")} />
          <Num label="H" value={lay.heightOf(n, box)} min={1} disabled={readOnly || sizing.h !== "fixed"} onChange={(v) => resize({ h: v }, "h")} />
          <Num label="↻" value={n.rotation ?? 0} min={-360} max={360} disabled={readOnly} suffix="°" onChange={(v) => patch({ rotation: v || undefined }, "rotation")} />
          <Num label="◐" value={Math.round((n.opacity ?? 1) * 100)} min={0} max={100} disabled={readOnly} suffix="%" onChange={(v) => patch({ opacity: v >= 100 ? undefined : v / 100 }, "opacity")} />
        </div>
      </div>

      {(n.type === "frame" || n.type === "component" || n.type === "instance") && <ComponentSection design={design} node={n} readOnly={readOnly} onEdit={onEdit} onSelect={onSelect} />}

      {container && (
        <Section title="Frame">
          <div className="grid grid-cols-2 gap-2">
            <label className={cx(field, "col-span-2")}>
              <span className="sr-only">Device preset</span>
              <select
                aria-label="Device preset"
                disabled={readOnly}
                className="w-full bg-transparent outline-none"
                value={"preset" in n ? (n.preset ?? "") : ""}
                onChange={(e) => {
                  const p = PRESETS.find((x) => x.id === e.target.value);
                  set((x) => {
                    if (x.type !== "frame" && x.type !== "component") return x;
                    const r = p ? (resizeNode(x, { x: x.x, y: x.y, w: p.w, h: p.h }) as typeof x) : x;
                    const out = { ...r, preset: p?.id };
                    if (!p) delete out.preset;
                    return out;
                  }, "preset");
                }}
              >
                <option value="">Custom size</option>
                {PRESETS.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.label} · {p.w}×{p.h}
                  </option>
                ))}
              </select>
            </label>
            <Num label="◜" value={n.radius ?? 0} min={0} disabled={readOnly} onChange={(v) => patch({ radius: v || undefined }, "radius")} />
            <label className="flex items-center gap-2 text-[12.5px] text-fg-2">
              <input type="checkbox" disabled={readOnly} checked={!!n.clip} onChange={(e) => patch({ clip: e.target.checked || undefined }, "clip")} />
              Clip content
            </label>
          </div>
        </Section>
      )}

      {container && (
        <Section
          title="Auto layout"
          action={
            !readOnly && (
              <button
                type="button"
                aria-label={al ? "Remove auto layout" : "Add auto layout"}
                className="grid size-6 place-items-center rounded-md text-fg-secondary hover:bg-hover"
                onClick={() => {
                  if (al) {
                    // Turning it off keeps children where the layout put them; hug becomes fixed.
                    set((x) => {
                      if (x.type !== "frame" && x.type !== "component") return x;
                      const out = { ...x, w: lay.widthOf(x), h: lay.heightOf(x, lay.widthOf(x)) };
                      delete out.auto_layout;
                      delete out.sizing;
                      out.children = x.children.map((c) => {
                        if (c.type === "connector" || !c.sizing?.w && !c.sizing?.h) return c;
                        if (c.sizing.w === "fill" || c.sizing.h === "fill") {
                          const s = { ...c.sizing };
                          if (s.w === "fill") delete s.w;
                          if (s.h === "fill") delete s.h;
                          return { ...c, sizing: Object.keys(s).length ? s : undefined } as DesignNode;
                        }
                        return c;
                      });
                      return out;
                    }, "al-off");
                  } else patch({ auto_layout: { direction: "vertical", gap: 16, padding: [16, 16, 16, 16] } }, "al-on");
                }}
              >
                {al ? "−" : "+"}
              </button>
            )
          }
        >
          {al ? (
            <div className="flex flex-col gap-2">
              <div className="flex gap-1 rounded-md border border-border p-0.5" role="radiogroup" aria-label="Direction">
                {(["vertical", "horizontal"] as const).map((dir) => (
                  <button key={dir} type="button" role="radio" aria-checked={al.direction === dir} disabled={readOnly} className={seg(al.direction === dir)} onClick={() => setAl({ direction: dir })}>
                    {dir === "vertical" ? "↓ Vertical" : "→ Horizontal"}
                  </button>
                ))}
              </div>
              <div className="grid grid-cols-2 gap-2">
                <Num label="Gap" value={al.gap ?? 0} min={0} disabled={readOnly || al.justify === "space-between"} onChange={(v) => setAl({ gap: v || undefined })} />
                <Num label="Pad" value={(al.padding ?? [0, 0, 0, 0])[0]} min={0} disabled={readOnly} onChange={(v) => setAl({ padding: [v, v, v, v] })} />
              </div>
              <div className="grid grid-cols-4 gap-1.5">
                {(["Top", "Right", "Bottom", "Left"] as const).map((side, i) => (
                  <Num key={side} label={side[0]} value={(al.padding ?? [0, 0, 0, 0])[i]} min={0} disabled={readOnly} onChange={(v) => {
                    const p = [...(al.padding ?? [0, 0, 0, 0])] as [number, number, number, number];
                    p[i] = v;
                    setAl({ padding: p });
                  }} />
                ))}
              </div>
              <div className="flex items-center gap-2">
                <span className="w-14 text-[11.5px] text-fg-tertiary">Align</span>
                <div className="flex flex-1 gap-1 rounded-md border border-border p-0.5" role="radiogroup" aria-label="Align">
                  {(["start", "center", "end"] as const).map((a) => (
                    <button key={a} type="button" role="radio" aria-checked={(al.align ?? "start") === a} disabled={readOnly} className={seg((al.align ?? "start") === a)} onClick={() => setAl({ align: a === "start" ? undefined : a })}>
                      {a[0].toUpperCase() + a.slice(1)}
                    </button>
                  ))}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-14 text-[11.5px] text-fg-tertiary">Justify</span>
                <select aria-label="Justify" disabled={readOnly} className={cx(field, "flex-1")} value={al.justify ?? "start"} onChange={(e) => setAl({ justify: e.target.value === "start" ? undefined : (e.target.value as AutoLayout["justify"]) })}>
                  <option value="start">Start</option>
                  <option value="center">Center</option>
                  <option value="end">End</option>
                  <option value="space-between">Space between</option>
                </select>
              </div>
            </div>
          ) : (
            <p className="text-[12px] text-fg-tertiary">Stack this frame’s layers with a gap and padding (⇧A).</p>
          )}
        </Section>
      )}

      {(autoLaid || canHug) && (
        <Section title="Resizing">
          <div className="grid grid-cols-2 gap-2">
            {(["w", "h"] as const).map((axis) => (
              <label key={axis} className={field}>
                <span className="w-3.5 text-[11px] text-fg-tertiary">{axis.toUpperCase()}</span>
                <select aria-label={axis === "w" ? "Width sizing" : "Height sizing"} disabled={readOnly} className="w-full bg-transparent outline-none" value={sizing[axis]} onChange={(e) => setSizing(axis, e.target.value)}>
                  <option value="fixed">Fixed</option>
                  {canHug && <option value="hug">Hug</option>}
                  {autoLaid && <option value="fill">Fill</option>}
                </select>
              </label>
            ))}
          </div>
        </Section>
      )}

      {!autoLaid && parent && (parent.type === "frame" || parent.type === "component") && (
        <Section title="Constraints">
          <div className="grid grid-cols-2 gap-2">
            <select aria-label="Horizontal constraint" disabled={readOnly} className={field} value={constraints.h ?? "left"} onChange={(e) => setConstraint("h", e.target.value)}>
              {["left", "right", "left-right", "center", "scale"].map((v) => <option key={v} value={v}>{v === "left-right" ? "Left & right" : v[0].toUpperCase() + v.slice(1)}</option>)}
            </select>
            <select aria-label="Vertical constraint" disabled={readOnly} className={field} value={constraints.v ?? "top"} onChange={(e) => setConstraint("v", e.target.value)}>
              {["top", "bottom", "top-bottom", "center", "scale"].map((v) => <option key={v} value={v}>{v === "top-bottom" ? "Top & bottom" : v[0].toUpperCase() + v.slice(1)}</option>)}
            </select>
          </div>
        </Section>
      )}

      {n.type === "shape" && (
        <Section title="Shape">
          <div className="grid grid-cols-2 gap-2">
            <select aria-label="Shape" disabled={readOnly} className={cx(field, "col-span-2")} value={n.shape} onChange={(e) => patch({ shape: e.target.value }, "shape")}>
              {SHAPE_PRESETS.map((s) => <option key={s} value={s}>{SHAPE_LABEL(s)}</option>)}
            </select>
            {n.shape !== "ellipse" && <Num label="◜" value={n.radius ?? (n.shape === "roundRect" ? 8 : 0)} min={0} disabled={readOnly} onChange={(v) => patch({ radius: v || undefined }, "radius")} />}
          </div>
        </Section>
      )}

      {n.type === "image" && (
        <Section title="Image">
          <div className="grid grid-cols-2 gap-2">
            <select aria-label="Image fit" disabled={readOnly} className={field} value={n.fit ?? "cover"} onChange={(e) => patch({ fit: e.target.value === "cover" ? undefined : e.target.value }, "fit")}>
              <option value="cover">Cover</option>
              <option value="contain">Contain</option>
              <option value="fill">Stretch</option>
            </select>
            <Num label="◜" value={n.radius ?? 0} min={0} disabled={readOnly} onChange={(v) => patch({ radius: v || undefined }, "radius")} />
            <label className={cx(field, "col-span-2")}>
              <span className="text-[11px] text-fg-tertiary">Alt</span>
              <input aria-label="Alt text" disabled={readOnly} maxLength={500} className="w-full bg-transparent outline-none" defaultValue={n.alt ?? ""} onBlur={(e) => patch({ alt: e.target.value.trim() || undefined }, "alt")} />
            </label>
          </div>
        </Section>
      )}

      {paragraphs && firstRun && (
        <Section title="Text">
          <div className="grid grid-cols-2 gap-2">
            <Num label="Aa" value={firstRun.size} min={4} max={400} disabled={readOnly} onChange={(v) => setText((ps) => mapRuns(ps, (r) => ({ ...r, size: v })), "size")} />
            <select aria-label="Weight" disabled={readOnly} className={field} value={firstRun.weight ?? (firstRun.bold ? 700 : 400)} onChange={(e) => setText((ps) => mapRuns(ps, (r) => {
              const out = { ...r, weight: Number(e.target.value) as Run["weight"] };
              delete out.bold;
              if (out.weight === 400) delete out.weight;
              return out;
            }), "weight")}>
              <option value={400}>Regular</option>
              <option value={500}>Medium</option>
              <option value={600}>Semibold</option>
              <option value={700}>Bold</option>
            </select>
            <div className="col-span-2 flex gap-1 rounded-md border border-border p-0.5" role="radiogroup" aria-label="Text align">
              {(["left", "center", "right", "justify"] as const).map((a) => (
                <button key={a} type="button" role="radio" aria-checked={(paragraphs[0].align ?? "left") === a} disabled={readOnly} className={seg((paragraphs[0].align ?? "left") === a)} onClick={() => setText((ps) => ps.map((p) => ({ ...p, align: a === "left" ? undefined : a })), "align")}>
                  {a[0].toUpperCase() + a.slice(1)}
                </button>
              ))}
            </div>
            <div className="col-span-2 flex gap-1">
              {(["italic", "underline", "strike"] as const).map((m) => (
                <button key={m} type="button" aria-pressed={!!firstRun[m]} disabled={readOnly} className={cx(seg(!!firstRun[m]), "border border-border")} onClick={() => setText((ps) => mapRuns(ps, (r) => ({ ...r, [m]: firstRun[m] ? undefined : true })), m)}>
                  {m === "italic" ? <i>I</i> : m === "underline" ? <u>U</u> : <s>S</s>}
                </button>
              ))}
            </div>
            <div className="col-span-2">
              <Color name="Text" value={firstRun.color} resolved={lay.color(firstRun.color ?? "$text", mode)} disabled={readOnly} onChange={(c) => setText((ps) => mapRuns(ps, (r) => ({ ...r, color: c })), "color")} />
            </div>
          </div>
        </Section>
      )}

      {(styled || n.type === "section") && (
        <Section title="Fill">
          <Color name="Fill" value={"fill" in n ? n.fill : undefined} resolved={"fill" in n ? (lay.color(typeof n.fill === "string" ? n.fill : undefined, mode) ?? undefined) : undefined} disabled={readOnly} onChange={(c) => patch({ fill: c }, "fill")} />
        </Section>
      )}

      {styled && (
        <Section title="Stroke">
          <div className="grid grid-cols-[1fr_88px] gap-2">
            <Color name="Stroke" value={"stroke" in n ? n.stroke : undefined} resolved={"stroke" in n ? lay.color(n.stroke, mode) : undefined} disabled={readOnly} onChange={(c) => patch({ stroke: c, stroke_width: c ? (("stroke_width" in n && n.stroke_width) || 1) : undefined }, "stroke")} />
            <Num label="≡" value={("stroke_width" in n && n.stroke_width) || (("stroke" in n && n.stroke) ? 1 : 0)} min={0} max={1000} disabled={readOnly || !("stroke" in n && n.stroke)} onChange={(v) => patch({ stroke_width: v }, "stroke_width")} />
          </div>
        </Section>
      )}
      <SendToTicket node={n} readOnly={readOnly} onEdit={onEdit} />
    </div>
  );
}
