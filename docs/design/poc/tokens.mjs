// Token snapshot from web/styles/tokens.css (Dusk). The production version is
// a build-time generator (web/lib/designs/tokens.generated.json) with a drift
// test; this PoC one shows the parse and the hash the sync badge compares.
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

const KINDS = [
  // [prefix test, kind]; anything else is skipped (fonts, shadows, layout sizes)
  [/^#[0-9a-f]{3,8}$/i, "color"],
  [/^rgba?\(/i, "color"],
  [/^-?\d+(\.\d+)?px$/, "dimension"],
];

function block(css, selector) {
  const i = css.indexOf(selector);
  if (i < 0) throw new Error(`no ${selector} block in tokens.css`);
  let depth = 0;
  const start = css.indexOf("{", i);
  for (let j = start; j < css.length; j++) {
    if (css[j] === "{") depth++;
    if (css[j] === "}" && --depth === 0) return css.slice(start + 1, j);
  }
  throw new Error(`unclosed ${selector}`);
}

/** #RGB, #RRGGBB(AA) and rgba() become upper-case #RRGGBB or #RRGGBBAA. */
function hex(v) {
  const m = v.match(/^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*([\d.]+)\s*)?\)$/i);
  if (m) {
    const h = (n) => Math.round(n).toString(16).padStart(2, "0");
    return ("#" + h(+m[1]) + h(+m[2]) + h(+m[3]) + (m[4] === undefined || +m[4] === 1 ? "" : h(+m[4] * 255))).toUpperCase();
  }
  if (/^#[0-9a-f]{3}$/i.test(v)) return ("#" + [...v.slice(1)].map((c) => c + c).join("")).toUpperCase();
  return v.toUpperCase();
}

function vars(body) {
  const out = {};
  for (const m of body.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/--([a-z0-9-]+)\s*:\s*([^;]+);/gi)) {
    const value = m[2].trim();
    const kind = KINDS.find(([re]) => re.test(value))?.[1];
    if (kind) out[m[1]] = { kind, value: kind === "color" ? hex(value) : value };
  }
  return out;
}

/** {hash, modes: {dark, light}}; light holds only the tokens it overrides. */
export function snapshot(path) {
  const css = readFileSync(path, "utf8");
  const dark = vars(block(css, ":root {"));
  const light = vars(block(css, ':root[data-theme="light"]'));
  return { hash: "sha256:" + createHash("sha256").update(css).digest("hex"), modes: { dark, light } };
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const s = snapshot(process.argv[2]);
  console.log(JSON.stringify({ hash: s.hash, dark: Object.keys(s.modes.dark).length, light: Object.keys(s.modes.light).length }));
}
