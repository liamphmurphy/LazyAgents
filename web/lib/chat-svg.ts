// SVG images in chat: an ```svg-image fence holds one <svg> document that the
// chat shows as an image (components/domain/svg-fence). The server stores the
// fence only for personas with the svg capability.

export const isSvgFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "svg-image";

const SVG_NS = "http://www.w3.org/2000/svg";

/** The document to draw: an image needs the namespace to parse at all, so a root element without one gets it. */
export function svgDocument(source: string): string {
  return source.trim().replace(/<svg\b(?![^>]*\sxmlns\s*=)/i, `<svg xmlns="${SVG_NS}"`);
}

/**
 * The data: URL an <img> shows the source through. Browsers render SVG in an
 * image with scripts off and no external loads, so the agent's markup can't
 * run or reach the network.
 */
export function svgDataUrl(source: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svgDocument(source))}`;
}

/** What an SVG's <title> says, for its alt text. */
export function svgTitle(source: string): string | undefined {
  return /<title[^>]*>([^<]+)<\/title>/i.exec(source)?.[1]?.trim() || undefined;
}

// The colors every visual shares. Charts use them directly; agent-written SVGs
// name them as var(--viz-*), swapped for real colors before drawing because an
// <img> can't see the page's CSS. The series order is the colorblind-safe
// order, so assign slots in sequence.

export type VizPalette = { surface: string; panel: string; ink: string; muted: string; line: string; grid: string; series: readonly string[] };

export const VIZ_LIGHT: VizPalette = {
  surface: "#FFFFFF", panel: "#F3F3F6", ink: "#17171A", muted: "#55555F", line: "#B9B9C3", grid: "#ECECEF",
  series: ["#2A78D6", "#EB6834", "#1BAF7A", "#EDA100", "#E87BA4", "#008300", "#6250D6", "#E34948"],
};

export const VIZ_DARK: VizPalette = {
  surface: "#111114", panel: "#1C1C22", ink: "#EDEDEF", muted: "#A1A1AA", line: "#4A4A55", grid: "#24242B",
  series: ["#3987E5", "#D95926", "#199E70", "#C98500", "#D55181", "#008300", "#9085E9", "#E66767"],
};

const VIZ_TOKEN = /var\(\s*--viz-([a-z0-9-]+)\s*\)/gi;

function vizColor(p: VizPalette, name: string): string | undefined {
  const slot = /^([1-8])(-soft)?$/.exec(name);
  if (slot) return p.series[Number(slot[1]) - 1] + (slot[2] ? "26" : "");
  return name === "surface" || name === "panel" || name === "ink" || name === "muted" || name === "line" ? p[name] : undefined;
}

/** Whether a source draws with the shared palette; one that doesn't keeps its white background. */
export const isThemedSvg = (source: string) => /var\(\s*--viz-/i.test(source);

export function themeSvg(source: string, palette: VizPalette): string {
  return source.replace(VIZ_TOKEN, (token, name: string) => vizColor(palette, name.toLowerCase()) ?? token);
}

const ROOT_OPEN = /<svg\b(?:[^>"']|"[^"]*"|'[^']*')*>/i;

function withStyle(doc: string, css: string): string {
  const root = ROOT_OPEN.exec(doc);
  if (!root || root[0].endsWith("/>")) return doc;
  const at = root.index + root[0].length;
  return `${doc.slice(0, at)}<style>${css}</style>${doc.slice(at)}`;
}

/** A self-contained copy for downloads and the Library, readable wherever it ends up. */
export function exportSvg(source: string): string {
  if (!isThemedSvg(source)) return svgDocument(source);
  return withStyle(svgDocument(themeSvg(source, VIZ_LIGHT)), `:root{background:${VIZ_LIGHT.surface}}`);
}

// Step-throughs: elements carry data-step="2" (from step 2 on) or "2-3" (only
// during those steps), and each frame hides the rest with CSS, so a step-through
// is still just images with nothing to run.

export const isStepsFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "svg-steps";

export const MAX_STEPS = 12;

const STEP_ATTR = /\bdata-step\s*=\s*["']([^"']*)["']/gi;
const STEP_VALUE = /^(\d+)(?:-(\d+))?$/;

function stepValues(source: string): { value: string; from: number; to: number }[] {
  return [...new Set([...source.matchAll(STEP_ATTR)].map((m) => m[1].trim()))].flatMap((value) => {
    const m = STEP_VALUE.exec(value);
    return m ? [{ value, from: Number(m[1]), to: m[2] ? Number(m[2]) : Infinity }] : [];
  });
}

/** A step-through fence: the SVG, then one numbered caption per step. */
export function parseStepsFence(code: string): { svg: string; captions: string[] } {
  const end = code.toLowerCase().lastIndexOf("</svg>");
  if (end < 0) return { svg: code, captions: [] };
  const close = end + "</svg>".length;
  const captions = code.slice(close).split("\n").map((l) => l.trim().replace(/^\d+[.)]\s*/, "")).filter(Boolean);
  return { svg: code.slice(0, close), captions };
}

export function stepCount(source: string, captions: readonly string[]): number {
  return Math.min(MAX_STEPS, Math.max(captions.length, ...stepValues(source).map((s) => s.from), 0));
}

export function stepFrame(source: string, step: number): string {
  const hidden = stepValues(source).filter((s) => step < s.from || step > s.to).map((s) => `[data-step="${s.value}"]`);
  return hidden.length ? withStyle(source, `${hidden.join(",")}{display:none}`) : source;
}
