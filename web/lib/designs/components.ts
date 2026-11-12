// Components and instances in the editor (ADR-designs §3.3), mirroring the
// server's create_component / instantiate / set_overrides / detach ops. An
// instance stores only its component id and overrides keyed by the main's
// descendant ids; layout expands it, so editing the main updates every
// instance on the next layout and overrides survive.
import { cloneFresh, find, kids, mapNode, newId } from "./edit.ts";
import type { Design, DesignNode, FrameNode, InstanceNode, Override } from "./schema.ts";

export type ComponentInfo = { id: string; name: string; page: string; instances: number };

const walk = (nodes: DesignNode[], f: (n: DesignNode) => void) => {
  for (const n of nodes) {
    f(n);
    walk(kids(n), f);
  }
};

/** The design's components, with how many instances each has. */
export function listComponents(d: Design): ComponentInfo[] {
  const out: ComponentInfo[] = [];
  const count = new Map<string, number>();
  for (const p of d.pages)
    walk(p.children, (n) => {
      if (n.type === "component") out.push({ id: n.id, name: n.name ?? "Component", page: p.id, instances: 0 });
      if (n.type === "instance") count.set(n.component, (count.get(n.component) ?? 0) + 1);
    });
  return out.map((c) => ({ ...c, instances: count.get(c.id) ?? 0 }));
}

/** Turns a frame into a component; null when the node isn't a frame. */
export function createComponent(d: Design, id: string, name?: string): Design | null {
  const f = find(d, id);
  if (!f || f.node.type !== "frame") return null;
  return mapNode(d, id, (n) => ({ ...(n as FrameNode), type: "component", ...(name ? { name } : {}) }));
}

/** A new instance of a component at (x, y) in parent; returns its id. */
export function instantiate(d: Design, component: string, parent: string, x: number, y: number): { design: Design; id: string } | null {
  const main = find(d, component)?.node;
  if (!main || main.type !== "component") return null;
  const inst: InstanceNode = { id: newId("n"), type: "instance", name: main.name, component, x, y, w: main.w, h: main.h, ...(main.sizing ? { sizing: main.sizing } : {}) };
  const put = (list: DesignNode[]) => [...list, inst];
  const next = d.pages.some((p) => p.id === parent)
    ? { ...d, pages: d.pages.map((p) => (p.id === parent ? { ...p, children: put(p.children) } : p)) }
    : mapNode(d, parent, (n) => ("children" in n ? ({ ...n, children: put(n.children) } as DesignNode) : n));
  return { design: next, id: inst.id };
}

/** Sets (or, with undefined values, clears) one descendant's override on an instance. */
export function setOverride(d: Design, instance: string, child: string, patch: Override): Design {
  return mapNode(d, instance, (n) => {
    if (n.type !== "instance") return n;
    const cur = { ...(n.overrides?.[child] ?? {}), ...patch };
    for (const k of Object.keys(cur) as (keyof Override)[]) if (cur[k] === undefined) delete cur[k];
    const overrides = { ...(n.overrides ?? {}) };
    if (Object.keys(cur).length) overrides[child] = cur;
    else delete overrides[child];
    const out: InstanceNode = { ...n, overrides };
    if (!Object.keys(overrides).length) delete out.overrides;
    return out;
  });
}

/** The main's descendants an instance can override: [id, name, type], in tree order. */
export function overridable(d: Design, instance: string): { id: string; name: string; type: DesignNode["type"] }[] {
  const n = find(d, instance)?.node;
  if (n?.type !== "instance") return [];
  const main = find(d, n.component)?.node;
  if (!main) return [];
  const out: { id: string; name: string; type: DesignNode["type"] }[] = [];
  walk(kids(main), (c) => out.push({ id: c.id, name: c.name ?? c.type, type: c.type }));
  return out;
}

/**
 * Replaces an instance with a frame holding a copy of its main's subtree (new
 * ids, overrides applied). The frame keeps the instance's id, so links and
 * connectors to it keep working. Editing the main no longer touches it.
 */
export function detach(d: Design, id: string): Design | null {
  const n = find(d, id)?.node;
  if (n?.type !== "instance") return null;
  const main = find(d, n.component)?.node;
  if (!main || main.type !== "component") return null;
  const ids = new Map<string, string>();
  const copy = cloneFresh(main, ids) as FrameNode;
  const byNew = new Map([...ids].map(([o, m]) => [m, o]));
  const apply = (c: DesignNode): DesignNode => {
    const o = n.overrides?.[byNew.get(c.id) ?? ""];
    let m = c as DesignNode & Record<string, unknown>;
    if (o) {
      m = { ...m };
      if (o.fill !== undefined) m.fill = o.fill;
      if (o.visible !== undefined) m.visible = o.visible;
      if (o.text !== undefined && m.type === "text") m.paragraphs = [{ ...m.paragraphs[0], runs: [{ ...m.paragraphs[0].runs[0], text: o.text }] }];
    }
    if ("children" in m && Array.isArray(m.children)) m = { ...m, children: m.children.map(apply) } as typeof m;
    return m;
  };
  const body = apply(copy) as FrameNode;
  const frame: FrameNode = { ...body, type: "frame", id, x: n.x, y: n.y, w: n.w, h: n.h, name: n.name ?? main.name };
  for (const k of ["rotation", "opacity", "visible", "locked", "sizing", "absolute", "constraints"] as const) {
    if (n[k] !== undefined) (frame as Record<string, unknown>)[k] = n[k];
    else delete (frame as Record<string, unknown>)[k];
  }
  return mapNode(d, id, () => frame);
}
