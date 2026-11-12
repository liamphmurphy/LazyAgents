// Display list -> SVG markup. The editor renders the same ops as React
// elements; the server renders this exact string to PNG with resvg. Text stays
// text: one <text> per measured piece, kerning off, so both renderers put every
// word where the layout measured it.
const esc = (s) => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
const DASH = { dash: [4, 3], dot: [1, 1], dashDot: [4, 3, 1, 3], longDash: [8, 3] };

function fillAttr(fill, defs, id) {
  if (!fill) return 'fill="none"';
  if (typeof fill === "string") {
    if (fill.length === 9) return `fill="${fill.slice(0, 7)}" fill-opacity="${(parseInt(fill.slice(7), 16) / 255).toFixed(3)}"`;
    return `fill="${fill}"`;
  }
  const a = (fill.angle * Math.PI) / 180;
  const gid = `g${id}`;
  defs.push(`<linearGradient id="${gid}" x1="${0.5 - Math.cos(a) / 2}" y1="${0.5 - Math.sin(a) / 2}" x2="${0.5 + Math.cos(a) / 2}" y2="${0.5 + Math.sin(a) / 2}"><stop offset="0" stop-color="${fill.stops[0]}"/><stop offset="1" stop-color="${fill.stops[1]}"/></linearGradient>`);
  return `fill="url(#${gid})"`;
}
function strokeAttr(op) {
  if (!op.stroke) return "";
  const s = op.stroke.length === 9 ? `stroke="${op.stroke.slice(0, 7)}" stroke-opacity="${(parseInt(op.stroke.slice(7), 16) / 255).toFixed(3)}"` : `stroke="${op.stroke}"`;
  const dash = op.dash && DASH[op.dash] ? ` stroke-dasharray="${DASH[op.dash].map((n) => n * op.sw).join(" ")}"` : "";
  return ` ${s} stroke-width="${op.sw}"${dash}${op.cap ? ` stroke-linecap="${op.cap}"` : ""}${op.join ? ` stroke-linejoin="${op.join}"` : ""}`;
}

/**
 * opts.assets: {assetId: dataUri}; opts.fontCss: @font-face rules for a browser
 * (resvg gets the font files directly); opts.background: page colour.
 */
export function toSvg({ box, ops }, opts = {}) {
  const defs = [];
  const body = [];
  const stack = [];
  let n = 0;
  const op_ = (o) => (o.opacity !== undefined && o.opacity < 1 ? ` opacity="${o.opacity}"` : "");
  for (const o of ops) {
    n++;
    switch (o.op) {
      case "rect":
        body.push(`<rect x="${o.x}" y="${o.y}" width="${o.w}" height="${o.h}"${o.r ? ` rx="${o.r}"` : ""} ${fillAttr(o.fill, defs, n)}${strokeAttr(o)}${op_(o)}/>`);
        break;
      case "ellipse":
        body.push(`<ellipse cx="${o.x + o.w / 2}" cy="${o.y + o.h / 2}" rx="${o.w / 2}" ry="${o.h / 2}" ${fillAttr(o.fill, defs, n)}${strokeAttr(o)}${op_(o)}/>`);
        break;
      case "path":
        body.push(`<path d="${o.d}" ${fillAttr(o.fill, defs, n)}${strokeAttr(o)}${op_(o)}/>`);
        break;
      case "polyline":
        body.push(`<polyline points="${o.points.map((p) => p.join(",")).join(" ")}" fill="none"${strokeAttr(o)} stroke-linejoin="round"${op_(o)}/>`);
        break;
      case "image": {
        const sx = o.w / o.src.w, sy = o.h / o.src.h;
        const cid = `c${n}`;
        defs.push(`<clipPath id="${cid}"><rect x="${o.x}" y="${o.y}" width="${o.w}" height="${o.h}"${o.r ? ` rx="${o.r}"` : ""}/></clipPath>`);
        body.push(`<g clip-path="url(#${cid})"${op_(o)}><image href="${opts.assets?.[o.asset] ?? ""}" x="${(o.x - o.src.x * sx).toFixed(2)}" y="${(o.y - o.src.y * sy).toFixed(2)}" width="${(o.iw * sx).toFixed(2)}" height="${(o.ih * sy).toFixed(2)}" preserveAspectRatio="none"><title>${esc(o.alt)}</title></image></g>`);
        break;
      }
      case "text":
        body.push(`<text x="${o.x}" y="${o.y}" font-family="Inter" font-size="${o.size}" font-weight="${o.weight}"${o.italic ? ' font-style="italic"' : ""} ${fillAttr(o.fill, defs, n)}${op_(o)}>${esc(o.text)}</text>`);
        break;
      case "clip": {
        const cid = `c${n}`;
        defs.push(`<clipPath id="${cid}"><rect x="${o.x}" y="${o.y}" width="${o.w}" height="${o.h}"${o.r ? ` rx="${o.r}"` : ""}/></clipPath>`);
        body.push(`<g clip-path="url(#${cid})">`);
        stack.push(cid);
        break;
      }
      case "unclip":
        body.push("</g>");
        stack.pop();
        break;
    }
  }
  const style = `text{font-kerning:none;font-feature-settings:"kern" 0;text-rendering:geometricPrecision}${opts.fontCss ?? ""}`;
  const bg = opts.background ? `<rect x="${box.x}" y="${box.y}" width="${box.w}" height="${box.h}" fill="${opts.background}"/>` : "";
  const size = opts.scale ? ` width="${box.w * opts.scale}" height="${box.h * opts.scale}"` : ` width="${box.w}" height="${box.h}"`;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${box.x} ${box.y} ${box.w} ${box.h}"${size}><style>${style}</style><defs>${defs.join("")}</defs>${bg}${body.join("")}</svg>`;
}
