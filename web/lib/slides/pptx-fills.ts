/**
 * Gradient fills for the PPTX export, which pptxgenjs can't write: a shape or
 * background is written with a placeholder solid fill, then swapped for an
 * a:gradFill here. No runtime imports, so unit tests load it directly.
 */
import type { Paint } from "./schema";

/** A two-stop a:gradFill; PowerPoint's angle runs clockwise from left-to-right, like ours, in 60000ths of a degree. */
export function gradFill(p: Exclude<Paint, string>, opacity?: number): string {
  const alpha = opacity !== undefined && opacity < 1 ? `<a:alpha val="${Math.round(opacity * 100000)}"/>` : "";
  const stop = (pos: number, c: string) => `<a:gs pos="${pos}"><a:srgbClr val="${c.replace("#", "").toUpperCase()}">${alpha}</a:srgbClr></a:gs>`;
  return `<a:gradFill rotWithShape="1"><a:gsLst>${stop(0, p.stops[0])}${stop(100000, p.stops[1])}</a:gsLst><a:lin ang="${Math.round(p.angle * 60000) % 21600000}" scaled="0"/></a:gradFill>`;
}

/**
 * Swaps the placeholder solid fills for gradients: a shape's is the first
 * a:solidFill in the p:spPr after its p:cNvPr (named by element id); a
 * background's is the one in p:bgPr.
 */
export function replaceFills(xml: string, gradients: { el?: string; xml: string }[]): string {
  for (const g of gradients) {
    const re = g.el ? new RegExp(`(<p:cNvPr id="\\d+" name="${g.el}">[\\s\\S]*?<p:spPr>[\\s\\S]*?)<a:solidFill>[\\s\\S]*?</a:solidFill>`) : /(<p:bgPr>)<a:solidFill>[\s\S]*?<\/a:solidFill>/;
    const next = xml.replace(re, (_, head: string) => head + g.xml);
    if (next === xml) throw new Error(`couldn't add a gradient${g.el ? ` to ${g.el}` : " to a background"}`);
    xml = next;
  }
  return xml;
}
