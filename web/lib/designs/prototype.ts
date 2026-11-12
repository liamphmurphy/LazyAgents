// Prototype links (ADR-designs §3.1 Link): the Prototype tab's edits, the
// wires on the canvas and Present mode's player. Pure, so it's unit-tested.
//   - A link whose target is no longer a top-level frame (removed by an edit
//     that merged in) is broken: the editor flags it, Present ignores it.
//   - Every edit returns the same design object when nothing changes.
import { ancestors, find, newId } from "./edit.ts";
import type { Design, DesignNode, Link } from "./schema.ts";

export type Trigger = Link["trigger"];
export type Transition = Link["transition"];
export const TRIGGERS: { id: Trigger; label: string }[] = [
  { id: "click", label: "On click" },
  { id: "hover", label: "On hover" },
  { id: "after-delay", label: "After delay" },
];
export const TRANSITIONS: { id: Transition; label: string }[] = [
  { id: "instant", label: "Instant" },
  { id: "dissolve", label: "Dissolve" },
  { id: "slide-left", label: "Slide left" },
  { id: "slide-right", label: "Slide right" },
  { id: "slide-up", label: "Slide up" },
  { id: "slide-down", label: "Slide down" },
  { id: "smart-animate", label: "Smart animate" },
];
export const DEFAULT_DURATION_MS = 300;
export const DEFAULT_DELAY_MS = 800;

export type Screen = { id: string; name: string; page: string; pageName: string };

/** Every top-level frame, in page order: what a link can go to and Present can start from. */
export function screens(d: Design): Screen[] {
  return d.pages.flatMap((p) => p.children.filter((n) => n.type === "frame").map((n) => ({ id: n.id, name: n.name ?? "Frame", page: p.id, pageName: p.name })));
}

/** The top-level node a node sits in (itself at the top). */
export function screenOf(d: Design, id: string): DesignNode | undefined {
  const f = find(d, id);
  if (!f) return undefined;
  return ancestors(d, id).at(-1) ?? f.node;
}

/** True when the link's hotspot exists and its target is a top-level frame. */
export function isLive(d: Design, l: Link, frames = new Set(screens(d).map((s) => s.id))): boolean {
  return frames.has(l.to) && !!find(d, l.from);
}

export const brokenLinks = (d: Design): Link[] => {
  const frames = new Set(screens(d).map((s) => s.id));
  return d.links.filter((l) => !isLive(d, l, frames));
};

export const linksFrom = (d: Design, nodeId: string) => d.links.filter((l) => l.from === nodeId);

/**
 * Adds a click link from a node, to the frame after its own screen (or the
 * first other frame). Undefined when there's no frame to go to.
 */
export function addLink(d: Design, from: string, id = newId("k")): { design: Design; id: string } | undefined {
  const all = screens(d);
  const own = screenOf(d, from)?.id;
  const at = all.findIndex((s) => s.id === own);
  const to = (at >= 0 ? [...all.slice(at + 1), ...all.slice(0, at)] : all).find((s) => s.id !== own) ?? all[0];
  if (!to) return undefined;
  const link: Link = { id, from, to: to.id, trigger: "click", transition: "dissolve", duration_ms: DEFAULT_DURATION_MS };
  return { design: { ...d, links: [...d.links, link] }, id };
}

/** Changes a link's fields; keeps delay_ms only for after-delay and duration only when animated. */
export function setLink(d: Design, id: string, patch: Partial<Omit<Link, "id">>): Design {
  let changed = false;
  const links = d.links.map((l) => {
    if (l.id !== id) return l;
    const n: Link = { ...l, ...patch };
    if (n.trigger === "after-delay") n.delay_ms ??= DEFAULT_DELAY_MS;
    else delete n.delay_ms;
    if (n.transition === "instant") delete n.duration_ms;
    else n.duration_ms ??= DEFAULT_DURATION_MS;
    changed = JSON.stringify(n) !== JSON.stringify(l);
    return changed ? n : l;
  });
  return changed ? { ...d, links } : d;
}

export function removeLink(d: Design, id: string): Design {
  const links = d.links.filter((l) => l.id !== id);
  return links.length === d.links.length ? d : { ...d, links };
}

// ---- Present mode.

/** The frame Present starts from: the wanted one when it's a frame, else the first frame of the page, else of the design. */
export function startScreen(d: Design, wanted?: string | null, pageId?: string | null): Screen | undefined {
  const all = screens(d);
  return all.find((s) => s.id === wanted) ?? all.find((s) => s.id === screenOf(d, wanted ?? "")?.id) ?? all.find((s) => s.page === pageId) ?? all[0];
}

/** The live links whose hotspot is in a frame, in the order they're drawn. */
export function hotspotsOf(d: Design, frame: string): Link[] {
  const frames = new Set(screens(d).map((s) => s.id));
  return d.links.filter((l) => isLive(d, l, frames) && screenOf(d, l.from)?.id === frame);
}

export type Player = { at: string; back: string[]; start: string };
export const play = (start: string): Player => ({ at: start, back: [], start });
export const follow = (p: Player, to: string): Player => (to === p.at ? p : { ...p, at: to, back: [...p.back, p.at].slice(-100) });
export const goBack = (p: Player): Player => (p.back.length ? { ...p, at: p.back.at(-1)!, back: p.back.slice(0, -1) } : p);
export const restart = (p: Player): Player => play(p.start);

/** The two layers' keyframes for a transition (the new frame's and the old one's). Smart animate dissolves for now. */
export function transitionFrames(t: Transition): { enter: Keyframe[]; leave: Keyframe[] } | undefined {
  const slide = (x: number, y: number) => ({
    enter: [{ transform: `translate(${x * 100}%, ${y * 100}%)` }, { transform: "translate(0, 0)" }],
    leave: [{ transform: "translate(0, 0)" }, { transform: `translate(${-x * 100}%, ${-y * 100}%)` }],
  });
  switch (t) {
    case "instant":
      return undefined;
    case "dissolve":
    case "smart-animate":
      return { enter: [{ opacity: 0 }, { opacity: 1 }], leave: [{ opacity: 1 }, { opacity: 1 }] };
    case "slide-left":
      return slide(1, 0);
    case "slide-right":
      return slide(-1, 0);
    case "slide-up":
      return slide(0, 1);
    case "slide-down":
      return slide(0, -1);
  }
}
