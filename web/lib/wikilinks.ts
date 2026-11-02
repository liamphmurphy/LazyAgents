// [[wikilinks]] in markdown (PLAN.md §12.2). Mirrors server/internal/links
// Parse: fenced code and inline code spans are skipped, "\|" inside tables
// stands for "|".

export type WikiRef = {
  raw: string; // whole [[…]]
  ref: string; // inside the brackets, as sent to /knowledge/resolve
  target: string;
  anchor: string;
  display: string;
  embed: boolean;
};

const WIKILINK = /(!?)\[\[([^[\]\n]+?)\]\]/g;
const FENCE = /^ {0,3}(```|~~~)/;
const CODE_SPAN = /`+[^`\n]*`+/g;

function split(inner: string) {
  inner = inner.replace(/\\\|/g, "|");
  let display = "";
  let anchor = "";
  const bar = inner.indexOf("|");
  if (bar >= 0) {
    display = inner.slice(bar + 1).trim();
    inner = inner.slice(0, bar);
  }
  const hash = inner.indexOf("#");
  if (hash >= 0) {
    anchor = inner.slice(hash + 1).trim();
    inner = inner.slice(0, hash);
  }
  return { target: inner.trim(), anchor, display };
}

/** Calls fn for each line outside fenced code, with code spans blanked. */
function eachLine(md: string, fn: (line: string, scan: string, i: number) => string | void): string[] {
  let fence = "";
  return md.split("\n").map((line, i) => {
    const f = FENCE.exec(line);
    if (f) {
      if (!fence) fence = f[1];
      else if (fence === f[1]) fence = "";
      return line;
    }
    if (fence) return line;
    const scan = line.replace(CODE_SPAN, (m) => " ".repeat(m.length));
    return fn(line, scan, i) ?? line;
  });
}

export function parseWikiLinks(md: string): WikiRef[] {
  const out: WikiRef[] = [];
  eachLine(md, (line, scan) => {
    for (const m of scan.matchAll(WIKILINK)) {
      const raw = line.slice(m.index!, m.index! + m[0].length);
      const inner = raw.slice(m[1].length + 2, -2);
      const s = split(inner);
      if (!s.target && !s.anchor) continue;
      out.push({ raw, ref: inner.replace(/\\\|/g, "|"), embed: m[1] === "!", ...s });
    }
  });
  return out;
}

export const WIKILINK_SCHEME = "wikilink:";

const escapeText = (s: string) => s.replace(/[\\[\]*_`]/g, (c) => `\\${c}`);

/** Text shown for a link when it has no "|display". */
export function defaultLabel(r: Pick<WikiRef, "target" | "anchor" | "display">) {
  if (r.display) return r.display;
  if (!r.target) return r.anchor;
  return r.anchor ? `${r.target} › ${r.anchor}` : r.target;
}

/**
 * Rewrites [[links]] into ordinary markdown links with a wikilink: URL, so
 * react-markdown renders them through the `a` component.
 */
export function linkify(md: string): string {
  return eachLine(md, (line, scan) => {
    let out = "";
    let last = 0;
    for (const m of scan.matchAll(WIKILINK)) {
      const raw = line.slice(m.index!, m.index! + m[0].length);
      const inner = raw.slice(m[1].length + 2, -2);
      const s = split(inner);
      if (!s.target && !s.anchor) continue;
      out += line.slice(last, m.index!) + `[${escapeText(defaultLabel(s))}](${WIKILINK_SCHEME}${encodeURIComponent(inner.replace(/\\\|/g, "|"))})`;
      last = m.index! + m[0].length;
    }
    return last ? out + line.slice(last) : line;
  }).join("\n");
}

/** The [[…]] text for a link target (paths for notes, ids for numbered things). */
export function linkTextFor(t: { kind: string; artifact_type?: string; path?: string; display_id?: string; title: string }) {
  if (t.kind === "artifact" && t.artifact_type === "note" && t.path) return t.path;
  return t.display_id ?? t.title;
}

/** Wiki URL for a note path. */
export function wikiHref(path: string) {
  return "/wiki/" + path.split("/").map(encodeURIComponent).join("/");
}
