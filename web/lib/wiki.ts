// Pure helpers for the wiki screens: page headers, the namespace tree and
// version diffs. Kept free of React and path aliases so node --test can load
// them directly.
import type { DiffLine } from "./diff";

// ------------------------------------------------------------ header

export type PageHeader = { topic: string; tags: string[]; body: string };

/**
 * Reads the "Topic:" / "Tags:" lines at the top of a page (mirrors the
 * server's wiki.ParseHeader). body is the rest; it's the original text when
 * there's no header. The header lines are blanked, not removed, so heading
 * anchors keep their source lines.
 */
export function pageHeader(md: string): PageHeader {
  const lines = md.replace(/\r\n/g, "\n").split("\n");
  let topic = "";
  const tags: string[] = [];
  let found = false;
  let i = 0;
  for (; i < lines.length; i++) {
    const l = lines[i].trim();
    if (l === "" && !found) continue;
    const m = /^(topic|tags)\s*:\s*(.*)$/i.exec(l);
    if (!m) break;
    found = true;
    if (m[1].toLowerCase() === "topic") topic = m[2].trim();
    else
      for (const t of m[2].split(",")) {
        const v = t.trim().replace(/^#/, "").toLowerCase();
        if (v && !tags.includes(v)) tags.push(v);
      }
  }
  if (!found) return { topic, tags, body: md };
  return { topic, tags, body: lines.map((l, j) => (j < i ? "" : l)).join("\n") };
}

// ------------------------------------------------------------ tree

export type TreePage = { id: string; path: string; title: string };
export type Folder<P extends TreePage = TreePage> = {
  name: string;
  path: string;
  folders: Map<string, Folder<P>>;
  pages: P[];
  page?: P;
  count: number; // pages inside, recursively, including its own page
};

/** Folders by path segment (case-insensitive); a page named like a folder becomes that folder's page. */
export function buildTree<P extends TreePage>(pages: P[], strip = ""): Folder<P> {
  const root: Folder<P> = { name: "", path: "", folders: new Map(), pages: [], count: 0 };
  for (const p of pages) {
    const rel = strip && p.path.toLowerCase().startsWith(strip.toLowerCase()) ? p.path.slice(strip.length) : p.path;
    const segs = rel.split("/");
    let f = root;
    segs.slice(0, -1).forEach((s, i) => {
      const key = s.toLowerCase();
      if (!f.folders.has(key)) f.folders.set(key, { name: s, path: strip + segs.slice(0, i + 1).join("/"), folders: new Map(), pages: [], count: 0 });
      f = f.folders.get(key)!;
    });
    f.pages.push(p);
  }
  // Pages named like a sibling folder ("Payments" next to "Payments/…").
  const adopt = (f: Folder<P>) => {
    f.pages = f.pages.filter((p) => {
      const leaf = p.path.split("/").pop()!.toLowerCase();
      const sub = f.folders.get(leaf);
      if (sub && !sub.page) {
        sub.page = p;
        return false;
      }
      return true;
    });
    f.folders.forEach(adopt);
  };
  adopt(root);
  const count = (f: Folder<P>): number => {
    let n = f.pages.length + (f.page ? 1 : 0);
    f.folders.forEach((c) => (n += count(c)));
    f.count = n;
    return n;
  };
  count(root);
  return root;
}

/** Folder paths that contain the current page (they're forced open). */
export function ancestors(current: string): string[] {
  const segs = current.split("/");
  return segs.slice(0, -1).map((_, i) => segs.slice(0, i + 1).join("/").toLowerCase());
}

// ------------------------------------------------------------ diff

export type NumberedLine = DiffLine & { a?: number; b?: number };
export type FoldedLine = NumberedLine | { op: "skip"; count: number; from: number; lines: NumberedLine[] };

/** Old (a) and new (b) line numbers for a line diff. */
export function numberLines(lines: DiffLine[]): NumberedLine[] {
  let a = 0;
  let b = 0;
  return lines.map((l) => (l.op === "same" ? { ...l, a: ++a, b: ++b } : l.op === "del" ? { ...l, a: ++a } : { ...l, b: ++b }));
}

/** Keeps ctx unchanged lines around each change; longer unchanged runs fold into a skip row. */
export function foldDiff(lines: NumberedLine[], ctx = 3): FoldedLine[] {
  const changed = lines.map((l) => l.op !== "same");
  const near = lines.map((_, i) => changed.slice(Math.max(0, i - ctx), i + ctx + 1).some(Boolean));
  const out: FoldedLine[] = [];
  let run: NumberedLine[] = [];
  const flush = (from: number) => {
    if (run.length) out.push({ op: "skip", count: run.length, from, lines: run });
    run = [];
  };
  lines.forEach((l, i) => {
    if (!near[i]) {
      run.push(l);
      return;
    }
    flush(i - run.length);
    out.push(l);
  });
  flush(lines.length - run.length);
  return out;
}

export function diffStats(lines: DiffLine[]) {
  return { added: lines.filter((l) => l.op === "add").length, removed: lines.filter((l) => l.op === "del").length };
}

/** "v2..v3" → ["v2", "v3"]; anything else → null. */
export function parseCompare(s: string | null): [string, string] | null {
  const m = /^\s*(\S+?)\.\.(\S+?)\s*$/.exec(s ?? "");
  return m ? [m[1], m[2]] : null;
}
