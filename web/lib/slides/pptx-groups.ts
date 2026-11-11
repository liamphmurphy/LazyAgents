/**
 * pptxgenjs has no group shapes, so the export writes a group's children as
 * ordinary shapes (named by element id, in order) and this wraps them in a
 * `p:grpSp` afterwards. The group's child offset and extent equal its own,
 * so the children keep their slide positions and PowerPoint and LibreOffice
 * open it as one group that still resizes as a unit.
 */
import type { Frame } from "./schema.ts";

/** EMUs per slide unit: 1920 units span 13.333 in, 914400 EMU per inch. */
export const EMU = 914400 / 144;

export type PptxGroup = { id: string; frame: Frame; children: string[] };

/** The top-level shapes in a slide's spTree that pptxgenjs writes (it never nests them). */
const OBJECT = /<p:(sp|pic|cxnSp|graphicFrame)\b[^>]*>[\s\S]*?<\/p:\1>/g;
const nameOf = (obj: string) => /<p:cNvPr\b[^>]*\bname="([^"]*)"/.exec(obj)?.[1];

/** A slide's XML with each group's children wrapped in a p:grpSp; a group with fewer than two shapes in the file is left as is. */
export function wrapGroups(xml: string, groups: PptxGroup[]): string {
  if (!groups.length) return xml;
  const objs = [...xml.matchAll(OBJECT)].map((m) => ({ start: m.index!, end: m.index! + m[0].length, name: nameOf(m[0]) }));
  // The largest cNvPr id on the slide, so the groups' ids are unique.
  let next = Math.max(1, ...[...xml.matchAll(/<p:cNvPr\b[^>]*\bid="(\d+)"/g)].map((m) => Number(m[1]))) + 1;
  const cuts: { start: number; end: number; open: string }[] = [];
  for (const g of groups) {
    const mine = objs.filter((o) => o.name && g.children.includes(o.name));
    // Children that drew nothing (an empty image slot, a missing image) aren't in the file.
    if (mine.length < 2) continue;
    const e = (v: number) => Math.round(v * EMU);
    const off = `x="${e(g.frame.x)}" y="${e(g.frame.y)}"`, ext = `cx="${e(g.frame.w)}" cy="${e(g.frame.h)}"`;
    const open =
      `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="${next++}" name="${g.id}"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
      `<p:grpSpPr><a:xfrm><a:off ${off}/><a:ext ${ext}/><a:chOff ${off}/><a:chExt ${ext}/></a:xfrm></p:grpSpPr>`;
    cuts.push({ start: mine[0].start, end: mine[mine.length - 1].end, open });
  }
  // From the end, so earlier offsets stay valid.
  let out = xml;
  for (const c of cuts.sort((a, b) => b.start - a.start)) out = out.slice(0, c.start) + c.open + out.slice(c.start, c.end) + "</p:grpSp>" + out.slice(c.end);
  return out;
}
