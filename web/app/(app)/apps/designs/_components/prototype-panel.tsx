"use client";

import { cx } from "@/lib/cx";
import { find, labelOf } from "@/lib/designs/edit";
import { addLink, brokenLinks, linksFrom, removeLink, screenOf, screens, setLink, TRANSITIONS, TRIGGERS, type Screen } from "@/lib/designs/prototype";
import type { Design, Link } from "@/lib/designs/schema";

type Props = {
  design: Design;
  selected: string[];
  readOnly: boolean;
  onEdit: (fn: (d: Design) => Design, key?: string) => void;
  onSelect: (ids: string[]) => void;
  /** Present from a frame (saves first). */
  onPresent: (frame?: string) => void;
};

const section = "border-b border-border-subtle px-4 py-3";
const label = "mb-2 text-11 font-semibold uppercase tracking-[0.08em] text-fg-tertiary";
const select = "h-[28px] w-full min-w-0 rounded-md border border-border bg-surface-sunken px-1.5 text-13 text-fg focus:border-accent disabled:opacity-50";
const small = "h-[26px] rounded-md border border-border px-2 text-12 text-fg hover:bg-hover disabled:opacity-50";

/** One link: trigger (and delay), target frame, transition (and duration), remove. */
function LinkEditor({ link, frames, broken, readOnly, onEdit, name }: { link: Link; frames: Screen[]; broken: boolean; readOnly: boolean; onEdit: Props["onEdit"]; name: string }) {
  const set = (patch: Partial<Link>, k: string) => onEdit((d) => setLink(d, link.id, patch), `link:${link.id}:${k}`);
  return (
    <li data-link={link.id} data-broken={broken || undefined} aria-label={`Interaction: ${name}`} className={cx("flex flex-col gap-1.5 rounded-lg border p-2", broken ? "border-pill-blocked-fg/60" : "border-border")}>
      {broken && (
        <p role="alert" className="text-12 text-pill-blocked-fg">
          Broken link: its target frame was deleted. Choose another frame or remove it; Present skips it.
        </p>
      )}
      <select aria-label="Trigger" className={select} disabled={readOnly} value={link.trigger} onChange={(e) => set({ trigger: e.target.value as Link["trigger"] }, "trigger")}>
        {TRIGGERS.map((t) => (
          <option key={t.id} value={t.id}>{t.label}</option>
        ))}
      </select>
      {link.trigger === "after-delay" && (
        <label className="flex items-center gap-2 text-12 text-fg-secondary">
          Delay
          <input aria-label="Delay (ms)" type="number" min={0} max={60000} step={100} disabled={readOnly} className={cx(select, "w-24")} value={link.delay_ms ?? 0} onChange={(e) => Number.isFinite(e.target.valueAsNumber) && set({ delay_ms: Math.min(60000, Math.max(0, Math.round(e.target.valueAsNumber))) }, "delay")} />
          ms
        </label>
      )}
      <label className="flex items-center gap-2 text-12 text-fg-secondary">
        <span className="w-[46px] flex-none">Go to</span>
        <select aria-label="Target frame" className={select} disabled={readOnly} value={broken ? "" : link.to} onChange={(e) => e.target.value && set({ to: e.target.value }, "to")}>
          {broken && <option value="">Deleted frame</option>}
          {frames.map((f) => (
            <option key={f.id} value={f.id}>{frames.some((o) => o.page !== f.page) ? `${f.pageName} / ${f.name}` : f.name}</option>
          ))}
        </select>
      </label>
      <label className="flex items-center gap-2 text-12 text-fg-secondary">
        <span className="w-[46px] flex-none">Animate</span>
        <select aria-label="Transition" className={select} disabled={readOnly} value={link.transition} onChange={(e) => set({ transition: e.target.value as Link["transition"] }, "transition")}>
          {TRANSITIONS.map((t) => (
            <option key={t.id} value={t.id}>{t.label}</option>
          ))}
        </select>
      </label>
      <div className="flex items-center gap-2">
        {link.transition !== "instant" && (
          <label className="flex items-center gap-2 text-12 text-fg-secondary">
            <input aria-label="Duration (ms)" type="number" min={0} max={10000} step={50} disabled={readOnly} className={cx(select, "w-20")} value={link.duration_ms ?? 0} onChange={(e) => Number.isFinite(e.target.valueAsNumber) && set({ duration_ms: Math.min(10000, Math.max(0, Math.round(e.target.valueAsNumber))) }, "duration")} />
            ms
          </label>
        )}
        <button type="button" disabled={readOnly} className={cx(small, "ml-auto text-pill-blocked-fg")} onClick={() => onEdit((d) => removeLink(d, link.id))}>
          Remove
        </button>
      </div>
    </li>
  );
}

/** The inspector's Prototype tab: the selected layer's interactions, or every link of the design. */
export function PrototypePanel({ design, selected, readOnly, onEdit, onSelect, onPresent }: Props) {
  const frames = screens(design);
  const live = new Set(frames.map((f) => f.id));
  const broken = new Set(brokenLinks(design).map((l) => l.id));
  const nameOf = (id: string) => {
    const n = find(design, id)?.node;
    return n ? labelOf(n) : "Deleted layer";
  };

  if (selected.length > 1) return <p className="px-4 py-6 text-13 text-fg-tertiary">Select one layer to add an interaction to it.</p>;

  if (selected.length === 1) {
    const id = selected[0];
    const node = find(design, id)?.node;
    if (!node) return null;
    const own = linksFrom(design, id);
    const screen = screenOf(design, id);
    const incoming = screen && live.has(screen.id) ? design.links.filter((l) => l.to === screen.id).length : 0;
    return (
      <>
        <section className={section} aria-label="Interactions">
          <div className="flex items-center justify-between">
            <h3 className={label}>Interactions</h3>
            <button
              type="button"
              aria-label="Add interaction"
              disabled={readOnly || !frames.length}
              title={frames.length ? undefined : "Add a frame to link to"}
              className="grid size-6 place-items-center rounded text-fg-secondary hover:bg-hover hover:text-fg disabled:opacity-40"
              onClick={() => onEdit((d) => addLink(d, id)?.design ?? d)}
            >
              +
            </button>
          </div>
          {own.length ? (
            <ul className="flex flex-col gap-2">
              {own.map((l) => (
                <LinkEditor key={l.id} link={l} frames={frames} broken={broken.has(l.id)} readOnly={readOnly} onEdit={onEdit} name={`${nameOf(l.from)} → ${live.has(l.to) ? nameOf(l.to) : "deleted frame"}`} />
              ))}
            </ul>
          ) : (
            <p className="text-13 text-fg-tertiary">{frames.length ? `Add an interaction to make “${labelOf(node)}” a hotspot that goes to another frame.` : "Add a frame first: interactions go to a top-level frame."}</p>
          )}
        </section>
        {screen && live.has(screen.id) && (
          <section className={section} aria-label="Flow">
            <h3 className={label}>Flow</h3>
            <p className="mb-2 text-13 text-fg-secondary">
              {screen.name ?? "Frame"} · {incoming} {incoming === 1 ? "link" : "links"} in
            </p>
            <button type="button" className={small} onClick={() => onPresent(screen.id)}>
              ▶ Present from “{screen.name ?? "Frame"}”
            </button>
          </section>
        )}
      </>
    );
  }

  return (
    <>
      <section className={section} aria-label="Flow">
        <h3 className={label}>Flow</h3>
        <p className="mb-2 text-13 text-fg-secondary">
          {frames.length} {frames.length === 1 ? "frame" : "frames"} · {design.links.length} {design.links.length === 1 ? "link" : "links"}
          {broken.size > 0 && <span className="text-pill-blocked-fg"> · {broken.size} broken</span>}
        </p>
        <button type="button" disabled={!frames.length} className={small} onClick={() => onPresent()}>
          ▶ Present
        </button>
      </section>
      <section className={section} aria-label="Links">
        <h3 className={label}>Links</h3>
        {design.links.length ? (
          <ul className="flex flex-col gap-1">
            {design.links.map((l) => (
              <li key={l.id} data-link={l.id} data-broken={broken.has(l.id) || undefined} className="flex items-center gap-1">
                <button type="button" className="min-w-0 flex-1 truncate rounded px-1 py-0.5 text-left text-13 text-fg hover:bg-hover" onClick={() => find(design, l.from) && onSelect([l.from])}>
                  {nameOf(l.from)} → {live.has(l.to) ? nameOf(l.to) : <span className="text-pill-blocked-fg">deleted frame (broken)</span>}
                </button>
                <button type="button" aria-label={`Remove link from ${nameOf(l.from)}`} disabled={readOnly} className="grid size-5 place-items-center rounded text-fg-tertiary hover:bg-hover hover:text-fg disabled:opacity-40" onClick={() => onEdit((d) => removeLink(d, l.id))}>
                  −
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-13 text-fg-tertiary">Select a layer and add an interaction to link it to a frame.</p>
        )}
      </section>
    </>
  );
}
