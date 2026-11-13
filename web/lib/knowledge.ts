// Pure helpers for the Knowledge app: the explorer tree, page addresses,
// the daily-note calendar, open tabs and outlines. Unit-tested in
// lib/knowledge.test.mjs.

export type TreeRow = { id: string; title: string; path: string; audience?: string; properties?: Record<string, unknown>; pending?: number };

export type TreeFolder = { kind: "folder"; name: string; path: string; folders: TreeFolder[]; pages: TreeRow[]; count: number };

/** Folders first, then pages, each alphabetical; a folder knows how many pages it holds in total. */
export function buildTree(rows: TreeRow[]): TreeFolder {
  const root: TreeFolder = { kind: "folder", name: "", path: "", folders: [], pages: [], count: 0 };
  for (const r of rows) {
    const segs = r.path.split("/").filter(Boolean);
    let at = root;
    at.count++;
    for (const seg of segs.slice(0, -1)) {
      const path = at.path ? `${at.path}/${seg}` : seg;
      let next = at.folders.find((f) => f.name === seg);
      if (!next) {
        next = { kind: "folder", name: seg, path, folders: [], pages: [], count: 0 };
        at.folders.push(next);
      }
      next.count++;
      at = next;
    }
    at.pages.push(r);
  }
  const sort = (f: TreeFolder) => {
    f.folders.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
    // Daily notes read newest first; everything else alphabetically.
    const daily = f.pages.length > 0 && f.pages.every((p) => /^\d{4}-\d{2}-\d{2}$/.test(p.title));
    f.pages.sort((a, b) => (daily ? b.title.localeCompare(a.title) : a.title.localeCompare(b.title, undefined, { numeric: true })));
    f.folders.forEach(sort);
  };
  sort(root);
  return root;
}

/** The folders that lead to a path, so the explorer can open them: "A/B/C" → ["A", "A/B"]. */
export function ancestors(path: string | null | undefined): string[] {
  const segs = (path ?? "").split("/").filter(Boolean);
  return segs.slice(0, -1).map((_, i) => segs.slice(0, i + 1).join("/"));
}

/** The same slug the server puts in page URLs (links.Slug): letters and digits, any script, joined by dashes. */
export function slug(title: string): string {
  return (title.toLowerCase().match(/[\p{L}\p{N}]+/gu) ?? []).join("-");
}

export const pageHref = (id: string, title = "") => `/knowledge/${id}${title ? `/${slug(title)}` : ""}`;

/** A month as weeks of day numbers, Monday first; null pads the first and last week. */
export function monthGrid(year: number, month: number): (number | null)[][] {
  const first = new Date(Date.UTC(year, month, 1));
  const days = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
  const lead = (first.getUTCDay() + 6) % 7;
  const cells: (number | null)[] = [...Array(lead).fill(null), ...Array.from({ length: days }, (_, i) => i + 1)];
  while (cells.length % 7) cells.push(null);
  const weeks: (number | null)[][] = [];
  for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7));
  return weeks;
}

/** A local date as YYYY-MM-DD, the daily-note title. */
export function isoDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export type Tab = { id: string; title: string; kind?: "page" | "graph" | "connect" | "home" };

/** Opens a tab after the active one, or focuses it if it's already open; titles follow renames. */
export function openTab(tabs: Tab[], tab: Tab, active?: string): Tab[] {
  const i = tabs.findIndex((t) => t.id === tab.id);
  if (i >= 0) return tabs[i].title === tab.title ? tabs : tabs.map((t, j) => (j === i ? { ...t, title: tab.title } : t));
  const at = tabs.findIndex((t) => t.id === active);
  const next = [...tabs];
  next.splice(at < 0 ? next.length : at + 1, 0, tab);
  return next.slice(-12);
}

/** Closes a tab; returns the tabs left and the one to show instead when it was active. */
export function closeTab(tabs: Tab[], id: string, active?: string): { tabs: Tab[]; next?: Tab } {
  const i = tabs.findIndex((t) => t.id === id);
  if (i < 0) return { tabs };
  const left = tabs.filter((t) => t.id !== id);
  if (id !== active) return { tabs: left };
  return { tabs: left, next: left[Math.min(i, left.length - 1)] };
}

/** Headings in Markdown outside fenced code, with their level and the slug read_page uses. */
export function outline(md: string): { level: number; text: string; slug: string }[] {
  const out: { level: number; text: string; slug: string }[] = [];
  let fence = "";
  for (const line of md.split("\n")) {
    const f = /^(```|~~~)/.exec(line.trim());
    if (f) {
      fence = fence ? (fence === f[1] ? "" : fence) : f[1];
      continue;
    }
    if (fence) continue;
    const m = /^(#{1,6})\s+(.*?)\s*#*\s*$/.exec(line);
    if (m) out.push({ level: m[1].length, text: m[2], slug: slug(m[2]) });
  }
  return out;
}

/** Words in a Markdown body, for the status bar. */
export const wordCount = (md: string) => (md.replace(/```[\s\S]*?```/g, " ").match(/[\p{L}\p{N}']+/gu) ?? []).length;

/** Frontmatter-style properties shown above a page, without the ones the app shows elsewhere. */
export function visibleProperties(p: Record<string, unknown>): [string, unknown][] {
  const hidden = new Set(["query", "zettel_id"]);
  return Object.entries(p).filter(([k, v]) => !hidden.has(k) && v !== null && v !== "" && !(Array.isArray(v) && v.length === 0));
}

/** "2 min ago", "Yesterday", "Nov 3": how recent pages read in lists. */
export function ago(iso: string, now = new Date()): string {
  const t = new Date(iso);
  const s = (now.getTime() - t.getTime()) / 1000;
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400 && t.getDate() === now.getDate()) return `${Math.floor(s / 3600)} h ago`;
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (t.toDateString() === y.toDateString()) return "Yesterday";
  return t.toLocaleDateString("en", { month: "short", day: "numeric", ...(t.getFullYear() !== now.getFullYear() ? { year: "numeric" } : {}) });
}

/**
 * The Docs editor's Markdown writer escapes brackets, which turns Obsidian
 * syntax into plain text: [[Page]] → \[\[Page\]\], ![[file]] → !\[\[file\]\],
 * > [!note] → > \[!note\]. Knowledge saves through this to keep links,
 * embeds and callouts working. Escapes inside code are left alone.
 */
export function restoreObsidianSyntax(md: string): string {
  const fix = (s: string) =>
    s
      .replace(/(!?)\\\[\\\[((?:(?!\\\]\\\]).)+?)\\\]\\\]/g, (_, bang: string, inner: string) => `${bang}[[${inner.replace(/\\([[\]|#^_*])/g, "$1")}]]`)
      .replace(/^(\s*>\s*)\\\[!([A-Za-z-]+)\\\]/gm, "$1[!$2]");
  // Leave fenced code and inline code exactly as written.
  return md
    .split(/(```[\s\S]*?```|~~~[\s\S]*?~~~)/)
    .map((part, i) => (i % 2 ? part : part.split(/(`[^`\n]*`)/).map((s, j) => (j % 2 ? s : fix(s))).join("")))
    .join("");
}
