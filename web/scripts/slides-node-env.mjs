// Lets the Slides export code (web/lib/slides) run under Node for the evidence
// scripts: fonts and assets are read from disk, and text is measured from the
// slide fonts' own advance widths (no kerning, as in the app).
import { readFileSync } from "node:fs";
import { createJiti } from "jiti";

export const WEB = new URL("../", import.meta.url).pathname;
export const FIX = new URL("../../docs/slides/fixtures/", import.meta.url).pathname;
export const jiti = createJiti(import.meta.url, { jsx: { runtime: "automatic" } });

// The exports fetch fonts from /fonts/slides; serve them from public/. Any
// other URL is an asset, read from the fixtures by its last path segment.
globalThis.fetch = async (url) => {
  const path = String(url).startsWith("/fonts/") ? WEB + "public" + url : FIX + String(url).split("/").pop();
  const b = readFileSync(path);
  return { ok: true, status: 200, arrayBuffer: async () => b.buffer.slice(b.byteOffset, b.byteOffset + b.byteLength) };
};

function advances(file) {
  const b = readFileSync(file), u16 = (o) => b.readUInt16BE(o), tables = {};
  for (let i = 0; i < u16(4); i++) tables[b.toString("latin1", 12 + i * 16, 16 + i * 16)] = b.readUInt32BE(20 + i * 16);
  const upm = u16(tables.head + 18), nh = u16(tables.hhea + 34), hmtx = tables.hmtx, cmap = tables.cmap;
  let sub;
  for (let i = 0; i < u16(cmap + 2); i++) if (u16(cmap + 4 + i * 8) === 3 && u16(cmap + 6 + i * 8) === 1) sub = cmap + b.readUInt32BE(cmap + 8 + i * 8);
  const segs = u16(sub + 6) / 2, ends = sub + 14, starts = ends + segs * 2 + 2, deltas = starts + segs * 2, ranges = deltas + segs * 2;
  const glyph = (c) => {
    for (let i = 0; i < segs; i++) {
      if (c > u16(ends + i * 2)) continue;
      if (c < u16(starts + i * 2)) return 0;
      const ro = u16(ranges + i * 2);
      if (!ro) return (c + u16(deltas + i * 2)) & 0xffff;
      const g = u16(ranges + i * 2 + ro + (c - u16(starts + i * 2)) * 2);
      return g ? (g + u16(deltas + i * 2)) & 0xffff : 0;
    }
    return 0;
  };
  return (text) => [...text].reduce((w, ch) => w + u16(hmtx + Math.min(glyph(ch.codePointAt(0)), nh - 1) * 4), 0) / upm;
}

export const FONT_FILES = [];
const FACES = {};
for (const [fam, file] of [["Sans", "LiberationSans"], ["Serif", "LiberationSerif"], ["Mono", "LiberationMono"]])
  for (const [style, suffix] of [["400 normal", "Regular"], ["700 normal", "Bold"], ["400 italic", "Italic"], ["700 italic", "BoldItalic"]]) {
    const path = `${WEB}public/fonts/slides/${file}-${suffix}.ttf`;
    FACES[`Vesper Slides ${fam}|${style}`] = advances(path);
    FONT_FILES.push({ family: `Vesper Slides ${fam}`, weight: style.split(" ")[0], style: style.split(" ")[1], path });
  }

globalThis.document = {
  createElement: () => ({
    getContext: () => {
      let face, px;
      return {
        set font(f) {
          const m = f.match(/^(italic )?(\d+) ([\d.]+)px "(.+)"$/);
          face = FACES[`${m[4]}|${m[2]} ${m[1] ? "italic" : "normal"}`];
          px = +m[3];
        },
        measureText: (t) => ({ width: face(t) * px }),
      };
    },
  }),
};
