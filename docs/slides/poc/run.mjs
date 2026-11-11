// Builds the acceptance deck three ways: SVG per slide (what the editor and
// presentation draw), a PDF (embedded fonts, vector text) and a PPTX (native,
// editable text boxes and shapes). Writes everything to ./out.
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { jsPDF } from "jspdf";
import PptxGenJS from "pptxgenjs";
import { layoutSlide, FAMILIES, fontFile, color, INSET, LINE, BULLET_INDENT } from "./layout.mjs";

const FIX = new URL("../fixtures/", import.meta.url).pathname;
const OUT = new URL("./out/", import.meta.url).pathname;
mkdirSync(OUT, { recursive: true });
const deck = JSON.parse(readFileSync(FIX + "acceptance-deck.json", "utf8"));
const assetBytes = Object.fromEntries(Object.entries(deck.assets).map(([id, a]) => [id, readFileSync(FIX + a.file)]));
const laid = deck.slides.map((s) => layoutSlide(deck, s));
const warnings = laid.flatMap((l) => l.warnings);

// ---------- SVG (editor / presentation preview) ----------
const esc = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;");
laid.forEach(({ ops }, i) => {
  const parts = ops.map((o, j) => {
    const t = o.rot ? ` transform="rotate(${o.rot.deg} ${o.rot.cx} ${o.rot.cy})"` : "";
    const paint = `fill="${o.fill ?? "none"}"${o.stroke ? ` stroke="${o.stroke}" stroke-width="${o.sw}"` : ""}`;
    switch (o.op) {
      case "rect": return `<rect x="${o.x}" y="${o.y}" width="${o.w}" height="${o.h}" rx="${o.r ?? 0}" ${paint}${t}/>`;
      case "ellipse": return `<ellipse cx="${o.x + o.w / 2}" cy="${o.y + o.h / 2}" rx="${o.w / 2}" ry="${o.h / 2}" ${paint}${t}/>`;
      case "line": return `<line x1="${o.x1}" y1="${o.y1}" x2="${o.x2}" y2="${o.y2}" stroke="${o.stroke}" stroke-width="${o.sw}"${o.arrow ? ' marker-end="url(#arrow)"' : ""}${t}/>`;
      case "image": {
        const a = deck.assets[o.asset], s = o.w / o.src.w;
        return `<clipPath id="c${j}"><rect x="${o.x}" y="${o.y}" width="${o.w}" height="${o.h}"/></clipPath><image clip-path="url(#c${j})" href="../../fixtures/${a.file}" x="${o.x - o.src.x * s}" y="${o.y - o.src.y * s}" width="${a.w * s}" height="${a.h * s}"${t}><title>${esc(o.alt)}</title></image>`;
      }
      case "text": return `<text x="${o.x}" y="${o.y}" font-family="${FAMILIES[o.family].web}" font-size="${o.size}" font-weight="${o.style.includes("bold") ? 700 : 400}" font-style="${o.style.includes("italic") ? "italic" : "normal"}" fill="${o.color}"${o.underline ? ' text-decoration="underline"' : ""} xml:space="preserve"${t}>${esc(o.text)}</text>`;
    }
  });
  writeFileSync(`${OUT}slide-${i + 1}.svg`, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${deck.size.w} ${deck.size.h}"><defs><marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="4" markerHeight="4" orient="auto"><path d="M0,0 L10,5 L0,10 z" fill="context-stroke"/></marker></defs>${parts.join("")}</svg>`);
});

// ---------- PDF: one 960 × 540 pt page per slide, fonts embedded ----------
const K = 0.5; // slide units → points
const pdf = new jsPDF({ unit: "pt", format: [deck.size.w * K, deck.size.h * K], orientation: "landscape", compress: true });
for (const [fam, f] of Object.entries(FAMILIES))
  for (const style of ["normal", "bold", "italic", "bolditalic"]) {
    const name = `${f.file}-${style}.ttf`;
    pdf.addFileToVFS(name, readFileSync(fontFile(fam, style)).toString("base64"));
    pdf.addFont(name, f.web, style);
  }
const hex = (c) => c;
laid.forEach(({ ops }, i) => {
  if (i) pdf.addPage([deck.size.w * K, deck.size.h * K], "landscape");
  for (const o of ops) {
    pdf.saveGraphicsState();
    if (o.rot) {
      // rotate about the element centre, in jsPDF's top-left page space
      const r = (o.rot.deg * Math.PI) / 180, cx = o.rot.cx * K, cy = deck.size.h * K - o.rot.cy * K;
      const c = Math.cos(r), s = Math.sin(r);
      // PDF space is y-up, so a clockwise slide rotation is a negative angle here
      pdf.setCurrentTransformationMatrix(new pdf.Matrix(c, -s, s, c, cx - c * cx - s * cy, cy + s * cx - c * cy));
    }
    const style = o.fill && o.stroke ? "FD" : o.fill ? "F" : "S";
    if (o.fill) pdf.setFillColor(hex(o.fill));
    if (o.stroke) { pdf.setDrawColor(hex(o.stroke)); pdf.setLineWidth(o.sw * K); }
    switch (o.op) {
      case "rect":
        if (o.r) pdf.roundedRect(o.x * K, o.y * K, o.w * K, o.h * K, o.r * K, o.r * K, style);
        else pdf.rect(o.x * K, o.y * K, o.w * K, o.h * K, style);
        break;
      case "ellipse": pdf.ellipse((o.x + o.w / 2) * K, (o.y + o.h / 2) * K, (o.w / 2) * K, (o.h / 2) * K, style); break;
      case "line": {
        pdf.setDrawColor(o.stroke); pdf.setLineWidth(o.sw * K);
        const head = o.arrow ? o.sw * 4 : 0;
        pdf.line(o.x1 * K, o.y1 * K, (o.x2 - head) * K, o.y2 * K);
        if (o.arrow) { pdf.setFillColor(o.stroke); pdf.triangle((o.x2 - head) * K, (o.y2 - head / 1.6) * K, o.x2 * K, o.y2 * K, (o.x2 - head) * K, (o.y2 + head / 1.6) * K, "F"); }
        break;
      }
      case "image": {
        const a = deck.assets[o.asset], s = o.w / o.src.w;
        pdf.rect(o.x * K, o.y * K, o.w * K, o.h * K, null); pdf.clip(); pdf.discardPath();
        pdf.addImage(new Uint8Array(assetBytes[o.asset]), "PNG", (o.x - o.src.x * s) * K, (o.y - o.src.y * s) * K, a.w * s * K, a.h * s * K, o.asset);
        break;
      }
      case "text":
        pdf.setFont(FAMILIES[o.family].web, o.style); pdf.setFontSize(o.size * K); pdf.setTextColor(o.color);
        pdf.text(o.text, o.x * K, o.y * K, { baseline: "alphabetic" });
        if (o.underline) { pdf.setDrawColor(o.color); pdf.setLineWidth(o.size * 0.06 * K); const u = (o.y + o.size * 0.12) * K; pdf.line(o.x * K, u, (o.x + o.w - (o.text.endsWith(" ") ? o.size * 0.28 : 0)) * K, u); }
        break;
    }
    pdf.restoreGraphicsState();
  }
});
pdf.setProperties({ title: "Quarterly review", creator: "Vesper Slides PoC" });
writeFileSync(OUT + "deck.pdf", Buffer.from(pdf.output("arraybuffer")));

// ---------- PPTX: native text boxes and shapes, no rasterised slides ----------
const IN = (u) => u / 144; // slide units → inches (1920 units = 13.333 in = LAYOUT_WIDE)
const PT = (u) => u * K;
const strip = (c) => c.replace("#", "");
const pptx = new PptxGenJS();
pptx.layout = "LAYOUT_WIDE";
pptx.title = "Quarterly review";
const officeFont = (r) => {
  const f = r.font ?? "body";
  return FAMILIES[f === "heading" || f === "body" ? deck.theme.fonts[f] : f].office;
};
function textRuns(box) {
  const runs = [];
  for (const p of box.paragraphs) {
    const maxSize = Math.max(...p.runs.map((r) => r.size));
    p.runs.forEach((r, i) => {
      runs.push({
        text: r.text,
        options: {
          fontFace: officeFont(r), fontSize: PT(r.size), bold: !!r.bold, italic: !!r.italic, underline: r.underline ? { style: "sng" } : undefined,
          color: strip(color(deck.theme, r.color)),
          // paragraph properties ride on every run of the paragraph
          align: p.align ?? "left", lineSpacing: PT(maxSize * LINE), paraSpaceBefore: PT(p.spaceBefore ?? 0),
          bullet: p.bullet ? (p.bullet === "number" ? { type: "number", indent: PT(BULLET_INDENT) } : { indent: PT(BULLET_INDENT) }) : false,
          indentLevel: p.bullet ? p.level ?? 0 : 0,
          breakLine: i === p.runs.length - 1,
        },
      });
    });
  }
  return runs;
}
const frameOpts = (e) => ({ x: IN(e.frame.x), y: IN(e.frame.y), w: IN(e.frame.w), h: IN(e.frame.h), rotate: e.rotation ?? 0 });
const textOpts = (box) => ({ margin: PT(INSET), valign: box.valign ?? "top", fit: "none", wrap: true });
for (const s of deck.slides) {
  const slide = pptx.addSlide();
  slide.background = { color: strip(color(deck.theme, s.background?.color, "background")) };
  if (s.notes) slide.addNotes(s.notes);
  for (const e of s.elements) {
    const shapeStyle = { fill: e.fill ? { color: strip(color(deck.theme, e.fill)) } : { type: "none" }, line: e.stroke ? { color: strip(color(deck.theme, e.stroke)), width: PT(e.strokeWidth) } : { type: "none" } };
    if (e.type === "text") slide.addText(textRuns(e), { ...frameOpts(e), ...textOpts(e), objectName: e.id });
    if (e.type === "shape") {
      const shape = { rect: pptx.ShapeType.rect, roundRect: pptx.ShapeType.roundRect, ellipse: pptx.ShapeType.ellipse }[e.shape];
      const radius = e.shape === "roundRect" ? IN(e.radius ?? 24) : undefined;
      if (e.text) slide.addText(textRuns(e.text), { shape, ...frameOpts(e), ...textOpts(e.text), ...shapeStyle, rectRadius: radius, objectName: e.id });
      else slide.addShape(shape, { ...frameOpts(e), ...shapeStyle, rectRadius: radius, objectName: e.id });
    }
    if (e.type === "line") slide.addShape(pptx.ShapeType.line, { ...frameOpts(e), line: { color: strip(color(deck.theme, e.stroke)), width: PT(e.strokeWidth), endArrowType: e.endArrow ? "triangle" : undefined }, objectName: e.id });
    if (e.type === "image") {
      const a = deck.assets[e.asset], f = frameOpts(e), src = e._src;
      // pptxgenjs measures the crop against the picture's w × h, so express the source rectangle in those inches
      slide.addImage({ data: `${a.mime};base64,${assetBytes[e.asset].toString("base64")}`, ...f, altText: a.alt, objectName: e.id,
        sizing: { type: "crop", x: (src.x / a.w) * f.w, y: (src.y / a.h) * f.h, w: (src.w / a.w) * f.w, h: (src.h / a.h) * f.h } });
    }
  }
}
await pptx.writeFile({ fileName: OUT + "deck.pptx" });
writeFileSync(OUT + "layout-report.json", JSON.stringify({ slides: laid.length, warnings, expectedCrops: Object.fromEntries(deck.slides.flatMap((s) => s.elements.filter((e) => e._src).map((e) => [e.id, { l: e._src.x / deck.assets[e.asset].w, t: e._src.y / deck.assets[e.asset].h, r: 1 - (e._src.x + e._src.w) / deck.assets[e.asset].w, b: 1 - (e._src.y + e._src.h) / deck.assets[e.asset].h }]))) }, null, 2));
console.log(JSON.stringify({ warnings }, null, 1));
