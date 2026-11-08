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
