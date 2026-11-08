// Pure logic for a reply's Sources block: lifting the trailing URL list out of the
// message, and telling the web pages a reply used from those a search merely found.
// Kept free of React so `npm run test:unit` covers it.

import { hostOf, type WebSearches, type WebSource } from "./web-search.ts";

export type ListedSource = { title: string; url: string };

const URL_RE = /\[([^\]]*)\]\((https?:\/\/[^)\s]+)\)|(https?:\/\/[^\s<>()\]]+)/gi;
// "Sources", "**References:**", "## Web sources", or "Sources: <urls>" on one line.
const HEADING = /^\s*(?:#{1,6}\s+)?[*_]*(?:web\s+)?(?:sources?|references?|links?|urls?)(?:\s+(?:i\s+)?(?:used|consulted))?[*_]*\s*(:?)\s*[*_]*\s*(.*)$/i;
const BULLET = /^\s*(?:[-*+]|\d+[.)])\s+/;
const RULE = /^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/;

const trimUrl = (url: string) => url.replace(/[.,;:!?'"*_]+$/, "");

/** The URLs on a line. A plain URL takes the text before it as its title. */
function lineSources(line: string): ListedSource[] {
  const out: ListedSource[] = [];
  for (const m of line.matchAll(URL_RE)) {
    if (m[2]) {
      out.push({ title: m[1].trim(), url: m[2] });
      continue;
    }
    const before = out.length ? "" : line.slice(0, m.index).replace(BULLET, "").replace(/[*_`<]/g, "").replace(/[\s:|(–—-]+$/, "").trim();
    out.push({ title: before, url: trimUrl(m[3]) });
  }
  return out;
}

/**
 * Splits a reply's closing list of URLs from the message. Only a headed list counts, and
 * every line under the heading must carry a URL, so ordinary prose with links stays put.
 */
export function splitSourceList(md: string): { body: string; listed: ListedSource[] } {
  const lines = md.split("\n");
  const collected: ListedSource[][] = [];
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i];
    if (!line.trim()) continue;
    const h = HEADING.exec(line);
    if (h && (!h[2] || h[1])) {
      const inline = lineSources(h[2]);
      // "Links: see below" is a sentence, not a heading.
      if (h[2] && !inline.length) break;
      const listed = [inline, ...collected.reverse()].flat();
      if (!listed.length) break;
      let end = i;
      while (end > 0 && (!lines[end - 1].trim() || RULE.test(lines[end - 1]))) end--;
      const body = lines.slice(0, end).join("\n");
      return body.trim() ? { body, listed } : { body: md, listed: [] };
    }
    const found = lineSources(line);
    if (!found.length) break;
    collected.push(found);
  }
  return { body: md, listed: [] };
}

function urlKey(url: string): string {
  try {
    const u = new URL(url);
    return `${u.hostname.replace(/^www\./, "")}${u.pathname.replace(/\/+$/, "")}${u.search}`.toLowerCase();
  } catch {
    return url;
  }
}

/**
 * The turn's web pages, split into those the reply used and the rest. Used means listed
 * at the end of the reply, or a search result the reply links to. Codex searches report
 * no results, so a listed URL stands on its own.
 */
export function usedWebSources(body: string, listed: ListedSource[], searches: WebSearches): { used: WebSource[]; more: WebSource[] } {
  const found = new Map(searches.sources.map((s) => [urlKey(s.url), s]));
  const used = new Map<string, WebSource>();
  for (const l of listed) {
    const key = urlKey(l.url);
    if (used.has(key)) continue;
    const hit = found.get(key);
    used.set(key, { title: hit && hit.title !== hit.host ? hit.title : l.title || hit?.title || hostOf(l.url), url: l.url, host: hostOf(l.url) });
  }
  for (const m of body.matchAll(URL_RE)) {
    const key = urlKey(m[2] ?? trimUrl(m[3]));
    const hit = found.get(key);
    if (hit && !used.has(key)) used.set(key, hit);
  }
  return { used: [...used.values()], more: searches.sources.filter((s) => !used.has(urlKey(s.url))) };
}

/** The muted half of the row: where the sources came from. */
export function sourceSplit(vesper: number, web: number): string {
  if (vesper && web) return `${vesper} in Vesper · ${web} from the web`;
  return vesper ? "in Vesper" : "from the web";
}
