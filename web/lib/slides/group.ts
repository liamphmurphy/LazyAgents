/**
 * Groups: elements that move, resize and restyle together. A group holds its
 * children in slide coordinates, so grouping and ungrouping never move
 * anything; its frame is the children's box, which the server derives the
 * same way (server/internal/deck Bounds). Pure functions; the editor wires
 * them to the selection.
 */
import { boxOf, scaleFrame, unionBox } from "./arrange.ts";
import type { Element, Frame, GroupElement, LeafElement } from "./schema.ts";

const round = (v: number) => Math.round(v * 100) / 100;

export const isGroup = (e: Element): e is GroupElement => e.type === "group";

/** The box around frames, as the server rounds it. */
export function boundsOf(frames: Frame[]): Frame {
  const b = unionBox(frames);
  if (!b) return { x: 0, y: 0, w: 0, h: 0 };
  return { x: round(b.l), y: round(b.t), w: round(b.r - b.l), h: round(b.b - b.t) };
}

/** A group with its frame refitted to its children. */
export const fit = (g: GroupElement): GroupElement => ({ ...g, frame: boundsOf(g.elements.map((c) => c.frame)) });

/** Every element, groups followed by their children. */
export const flatten = (elements: Element[]): Element[] => elements.flatMap((e) => (isGroup(e) ? [e, ...e.elements] : [e]));

/** The element with an id anywhere on the slide, and the group it's in. */
export function findDeep(elements: Element[], id: string): { el: Element; group?: GroupElement } | undefined {
  for (const e of elements) {
    if (e.id === id) return { el: e };
    if (isGroup(e)) {
      const c = e.elements.find((x) => x.id === id);
      if (c) return { el: c, group: e };
    }
  }
  return undefined;
}

/** The elements a selection lives among: the slide's, or an entered group's children. */
export function scopeElements(elements: Element[], scope?: string): Element[] {
  const g = scope ? elements.find((e) => e.id === scope) : undefined;
  return g && isGroup(g) ? g.elements : elements;
}

/** A group moved and resized to `to`, its children scaled with it. */
export function scaleGroup(g: GroupElement, to: Frame): GroupElement {
  const from = boxOf(g.frame), dest = boxOf(to);
  return fit({ ...g, elements: g.elements.map((c) => ({ ...c, frame: scaleFrame(c.frame, from, dest) })) });
}

/** An element given a new frame; a group scales its children to it. */
export const withFrame = (e: Element, frame: Frame): Element => (isGroup(e) ? scaleGroup(e, frame) : { ...e, frame });

/**
 * The elements with fn applied to those with the ids, inside groups too.
 * Groups whose children changed are refitted. The same array when nothing
 * changes.
 */
export function mapDeep(elements: Element[], ids: ReadonlySet<string>, fn: (e: Element) => Element): Element[] {
  let changed = false;
  const out = elements.map((e) => {
    let n = ids.has(e.id) ? fn(e) : e;
    if (isGroup(n)) {
      const g = n;
      const kids = g.elements.map((c) => (ids.has(c.id) ? (fn(c) as LeafElement) : c));
      if (kids.some((c, i) => c !== g.elements[i])) n = fit({ ...g, elements: kids });
    }
    if (n !== e) changed = true;
    return n;
  });
  return changed ? out : elements;
}

/** The elements without the ids, inside groups too; a group left with one element dissolves into it. */
export function removeDeep(elements: Element[], ids: ReadonlySet<string>): Element[] {
  return elements.flatMap((e): Element[] => {
    if (ids.has(e.id)) return [];
    if (!isGroup(e) || !e.elements.some((c) => ids.has(c.id))) return [e];
    const kids = e.elements.filter((c) => !ids.has(c.id));
    return kids.length >= 2 ? [fit({ ...e, elements: kids })] : kids;
  });
}

/**
 * The selected top-level elements made into one group, in their stacking
 * order, where the topmost of them was. Selected groups are merged in (groups
 * don't nest) and children drop their layout placeholder role. Undefined
 * when fewer than two elements would be grouped.
 */
export function groupElements(elements: Element[], ids: ReadonlySet<string>, newId: () => string): { elements: Element[]; id: string } | undefined {
  const picked = elements.filter((e) => ids.has(e.id));
  const kids: LeafElement[] = picked.flatMap((e) => (isGroup(e) ? e.elements : [e])).map((c) => (c.placeholder ? { ...c, placeholder: undefined } : c));
  if (kids.length < 2) return undefined;
  const g = fit({ id: newId(), type: "group", frame: { x: 0, y: 0, w: 0, h: 0 }, elements: kids });
  const top = elements.reduce((at, e, i) => (ids.has(e.id) ? i : at), -1);
  const out = elements.flatMap((e, i) => (i === top ? [g] : ids.has(e.id) ? [] : [e]));
  return { elements: out, id: g.id };
}

/** The selected groups replaced by their children, in place; the children's ids to select. */
export function ungroupElements(elements: Element[], ids: ReadonlySet<string>): { elements: Element[]; ids: string[] } {
  const freed: string[] = [];
  const out = elements.flatMap((e) => {
    if (!ids.has(e.id) || !isGroup(e)) return [e];
    freed.push(...e.elements.map((c) => c.id));
    return e.elements;
  });
  return freed.length ? { elements: out, ids: freed } : { elements, ids: [] };
}
