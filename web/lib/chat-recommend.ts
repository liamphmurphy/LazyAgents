// Recommendation cards in chat: a ```recommend fence holds a URL and a one-line
// reason (components/domain/recommend-fence). Kept free of React so
// `npm run test:unit` covers it.

export const isRecommendFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "recommend";

export type Recommendation = { url: string; note: string };

/** The fence's URL and reason, or null when it doesn't start with a usable link. */
export function parseRecommend(code: string): Recommendation | null {
  const [first, ...rest] = code.trim().split("\n");
  const url = first.trim();
  try {
    const u = new URL(url);
    if (u.protocol !== "https:" && u.protocol !== "http:") return null;
  } catch {
    return null;
  }
  return { url, note: rest.join(" ").replace(/\s+/g, " ").trim() };
}

/** The privacy-mode player, or null for anything but a well-formed id. */
export function videoEmbedUrl(id: string | undefined): string | null {
  return id && /^[A-Za-z0-9_-]{11}$/.test(id) ? `https://www.youtube-nocookie.com/embed/${id}?autoplay=1&rel=0` : null;
}

type Node = { type: string; tagName?: string; value?: string; properties?: Record<string, unknown>; children?: Node[] };

const isCard = (n: Node) => n.type === "element" && n.tagName === "pre" && n.children?.some((c) => c.tagName === "code" && (c.properties?.className as string[] | undefined)?.includes("language-recommend"));

export const RECOMMEND_ROW = "recommend-row";

/** Rehype plugin: neighbouring recommend fences share a row, side by side. */
export function groupRecommendFences() {
  return (tree: Node) => {
    const out: Node[] = [];
    let run: Node[] = [];
    let gap: Node[] = [];
    const flush = () => {
      if (run.length > 1) out.push({ type: "element", tagName: "div", properties: { className: [RECOMMEND_ROW] }, children: run });
      else out.push(...run);
      out.push(...gap);
      run = [];
      gap = [];
    };
    for (const n of tree.children ?? []) {
      if (isCard(n)) {
        gap = [];
        run.push(n);
      } else if (run.length && n.type === "text" && !n.value?.trim()) {
        gap.push(n);
      } else {
        flush();
        out.push(n);
      }
    }
    flush();
    tree.children = out;
  };
}
