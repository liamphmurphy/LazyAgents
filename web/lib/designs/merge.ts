// Three-way merge of design bodies (ADR-designs §3.6): per node, page, link,
// asset and design setting, plus the order of each container's children. A
// change on one side only wins; the same change on both sides is fine;
// different changes to the same thing are a conflict, and nothing is
// overwritten silently.
import { canonical, normalize } from "./schema.ts";
import type { Design, DesignNode, Page } from "./schema.ts";

const PAGES = "$pages";
type Side = { body: Map<string, string>; home: Map<string, string>; lists: Map<string, string[]>; nodes: Map<string, DesignNode>; pages: Map<string, Page> };

const childList = (n: DesignNode) => ("children" in n && Array.isArray(n.children) ? n.children : undefined);

function index(d: Design): Side {
  const s: Side = { body: new Map(), home: new Map(), lists: new Map(), nodes: new Map(), pages: new Map() };
  s.body.set("$meta", canonical({ mode: d.mode, tokens_ref: d.tokens_ref ?? null, token_sets: d.token_sets }));
  s.lists.set(PAGES, d.pages.map((p) => p.id));
  for (const p of d.pages) {
    s.body.set(p.id, canonical({ ...p, children: undefined }));
    s.home.set(p.id, PAGES);
    s.pages.set(p.id, p);
    const walk = (list: DesignNode[], home: string) => {
      s.lists.set(home, list.map((n) => n.id));
      for (const n of list) {
        const { children, ...rest } = n as DesignNode & { children?: DesignNode[] };
        s.body.set(n.id, canonical(rest));
        s.home.set(n.id, home);
        s.nodes.set(n.id, n);
        if (Array.isArray(children)) walk(children, n.id);
      }
    };
    walk(p.children, p.id);
  }
  for (const l of d.links) s.body.set(`link:${l.id}`, canonical(l));
  for (const [k, a] of Object.entries(d.assets)) s.body.set(`asset:${k}`, canonical(a));
  return s;
}

const state = (s: Side, id: string) => (s.body.has(id) ? `${s.home.get(id) ?? ""}|${s.body.get(id)}` : "");
const same = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);

export type MergeResult = { design: Design; conflicts: [] } | { design?: undefined; conflicts: string[] };

/** Merges mine and theirs, both edited from base. Conflicts name the ids both sides changed differently. */
export function mergeDesigns(base: Design, mine: Design, theirs: Design): MergeResult {
  const b = index(base), m = index(mine), t = index(theirs);
  const conflicts: string[] = [];
  // Which side each entity comes from (absent: removed).
  const from = new Map<string, Side>();
  for (const id of new Set([...b.body.keys(), ...m.body.keys(), ...t.body.keys()])) {
    const sb = state(b, id), sm = state(m, id), st = state(t, id);
    const side = sm === st || sm === sb ? t : st === sb ? m : undefined;
    if (!side) conflicts.push(id);
    else if (side.body.has(id)) from.set(id, side);
  }
  if (conflicts.length) return { conflicts };

  // Each home's order: the side that reordered the survivors wins, then additions go after their previous sibling.
  const homes = new Set<string>();
  for (const [id, side] of from) if (side.home.has(id)) homes.add(side.home.get(id)!);
  const lists = new Map<string, string[]>();
  for (const home of homes) {
    const here = (id: string) => from.get(id)?.home.get(id) === home;
    const shared = (s: Side) => (s.lists.get(home) ?? []).filter((id) => here(id) && b.home.get(id) === home && m.home.get(id) === home && t.home.get(id) === home);
    const bs = shared(b), ms = shared(m), ts = shared(t);
    let seq: string[];
    if (same(ms, bs) || same(ms, ts)) seq = ts;
    else if (same(ts, bs)) seq = ms;
    else {
      conflicts.push(home);
      continue;
    }
    seq = [...seq];
    for (const s of [m, t]) {
      const src = s.lists.get(home) ?? [];
      src.forEach((id, i) => {
        if (seq.includes(id) || !here(id) || from.get(id) !== s) return;
        let at = 0;
        for (let k = i - 1; k >= 0; k--) {
          const j = seq.indexOf(src[k]);
          if (j >= 0) {
            at = j + 1;
            break;
          }
        }
        seq.splice(at, 0, id);
      });
    }
    lists.set(home, seq);
  }
  if (conflicts.length) return { conflicts };

  const build = (home: string): DesignNode[] =>
    (lists.get(home) ?? []).map((id) => {
      const n = from.get(id)!.nodes.get(id)!;
      return childList(n) ? ({ ...n, children: build(id) } as DesignNode) : n;
    });
  const pages = (lists.get(PAGES) ?? []).map((id) => ({ ...from.get(id)!.pages.get(id)!, children: build(id) }));

  // Every surviving node must have landed: one whose parent the other side removed needs the person.
  const placed = new Set<string>();
  const mark = (list: DesignNode[]) => {
    for (const n of list) {
      placed.add(n.id);
      const ch = childList(n);
      if (ch) mark(ch);
    }
  };
  pages.forEach((p) => mark(p.children));
  const orphans = [...from.keys()].filter((id) => from.get(id)!.nodes.has(id) && !placed.has(id));
  if (orphans.length) return { conflicts: orphans };

  const pickDesign = (s: Side) => (s === m ? mine : theirs);
  const links = [...theirs.links, ...mine.links.filter((l) => !theirs.links.some((x) => x.id === l.id))]
    .filter((l) => from.has(`link:${l.id}`))
    .map((l) => pickDesign(from.get(`link:${l.id}`)!).links.find((x) => x.id === l.id)!);
  const assets: Design["assets"] = {};
  for (const k of new Set([...Object.keys(theirs.assets), ...Object.keys(mine.assets)])) {
    const s = from.get(`asset:${k}`);
    if (s) assets[k] = pickDesign(s).assets[k];
  }
  const meta = pickDesign(from.get("$meta")!);
  const out: Design = { ...theirs, mode: meta.mode, tokens_ref: meta.tokens_ref, token_sets: meta.token_sets, pages, links, assets };
  if (!out.tokens_ref) delete out.tokens_ref;
  // A merge the validator refuses (a connector to a node the other side removed…) needs the person.
  try {
    return { design: normalize(out).design, conflicts: [] };
  } catch {
    return { conflicts: ["$design"] };
  }
}
