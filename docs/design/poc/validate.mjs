// Validates a design body: the JSON Schema (structure) plus the semantic rules
// in ADR-designs.md §3.1 that JSON Schema can't express. The production
// validators (server/internal/design in Go, web/lib/designs/schema.ts) implement
// the same rules and share invalid-body test cases.
//   node validate.mjs <design.json> [...]
import Ajv2020 from "ajv/dist/2020.js";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const schema = JSON.parse(readFileSync(join(here, "../schema/design-1.schema.json"), "utf8"));

export const CAPS = { body: 2 * 1024 * 1024, nodes: 10000, depth: 32, textChars: 5000 };

const ajv = new Ajv2020({ allErrors: true, strict: false });
ajv.addFormat("date-time", (s) => !Number.isNaN(Date.parse(s)) && /T/.test(s));
const structural = ajv.compile(schema);

const CONTAINERS = new Set(["frame", "component", "group", "section"]);

export function walk(design, fn) {
  const visit = (node, parent, page, depth) => {
    fn(node, parent, page, depth);
    for (const c of node.children ?? []) visit(c, node, page, depth + 1);
  };
  for (const p of design.pages) for (const n of p.children) visit(n, null, p, 1);
}

/** Returns a list of problems; empty means valid. */
export function validate(design, raw = JSON.stringify(design)) {
  const errs = [];
  if (Buffer.byteLength(raw) > CAPS.body) errs.push(`body is ${Buffer.byteLength(raw)} bytes; the cap is ${CAPS.body}`);
  if (!structural(design)) {
    for (const e of structural.errors.slice(0, 20)) errs.push(`schema: ${e.instancePath || "/"} ${e.message}`);
    return errs;
  }
  const ids = new Map(); // id -> {node, page, parent}
  const seen = new Set();
  const dup = (id) => (seen.has(id) ? errs.push(`duplicate id ${id}`) : seen.add(id));
  for (const p of design.pages) dup(p.id);
  for (const l of design.links) dup(l.id);
  let count = 0;
  walk(design, (n, parent, page, depth) => {
    dup(n.id);
    ids.set(n.id, { n, page, parent });
    count++;
    if (depth > CAPS.depth) errs.push(`${n.id} is nested ${depth} deep; the cap is ${CAPS.depth}`);
    if (parent && !CONTAINERS.has(parent.type)) errs.push(`${n.id}: a ${parent.type} can't have children`);
    if (n.type === "connector" && parent && parent.type === "group") errs.push(`${n.id}: connectors can't sit in a group`);
    const chars = (n.paragraphs ?? n.text?.paragraphs ?? []).flatMap((p) => p.runs).reduce((a, r) => a + r.text.length, 0);
    if (chars > CAPS.textChars) errs.push(`${n.id} has ${chars} characters of text; the cap is ${CAPS.textChars}`);
    if (n.type === "image" && !design.assets[n.asset]) errs.push(`${n.id}: asset ${n.asset} isn't in assets`);
    if (n.type === "image" && n.crop && (n.crop.l + n.crop.r >= 1 || n.crop.t + n.crop.b >= 1)) errs.push(`${n.id}: crop leaves nothing`);
    if (n.sizing && Object.values(n.sizing).includes("fill") && !parent?.auto_layout) errs.push(`${n.id}: fill sizing needs a parent with auto layout`);
    if (n.sizing && Object.values(n.sizing).includes("hug") && !["text", "instance"].includes(n.type) && !n.auto_layout) errs.push(`${n.id}: hug sizing needs auto layout or text`);
  });
  if (count > CAPS.nodes) errs.push(`${count} nodes; the cap is ${CAPS.nodes}`);

  // References.
  const set = design.token_sets[design.tokens_ref?.set];
  if (design.tokens_ref && !set) errs.push(`tokens_ref.set ${design.tokens_ref.set} isn't in token_sets`);
  const tokenOk = (name, kind) => {
    const t = set?.modes.dark[name];
    return t && t.kind === kind;
  };
  const colors = [];
  const collect = (v, where) => {
    if (typeof v === "string") colors.push([v, where]);
    else if (v && typeof v === "object" && Array.isArray(v.stops)) v.stops.forEach((s) => colors.push([s, where]));
  };
  for (const p of design.pages) collect(p.background, p.id);
  for (const [id, { n }] of ids) {
    for (const k of ["fill", "stroke"]) collect(n[k], id);
    for (const p of n.paragraphs ?? n.text?.paragraphs ?? []) for (const r of p.runs) (collect(r.color, id), collect(r.highlight, id));
    for (const o of Object.values(n.overrides ?? {})) collect(o.fill, id);
  }
  for (const [c, where] of colors) if (c.startsWith("$") && !tokenOk(c.slice(1), "color")) errs.push(`${where}: token ${c} isn't a colour in ${design.tokens_ref?.set ?? "(no token set)"}`);

  for (const [id, { n, page }] of ids) {
    if (n.type === "instance") {
      const main = ids.get(n.component)?.n;
      if (!main || main.type !== "component") errs.push(`${id}: component ${n.component} isn't a component`);
      else {
        const inMain = new Set();
        walk({ pages: [{ children: [main] }] }, (d) => inMain.add(d.id));
        for (const k of Object.keys(n.overrides ?? {})) if (!inMain.has(k) || k === main.id) errs.push(`${id}: override ${k} isn't inside ${main.id}`);
        if (inMain.has(id)) errs.push(`${id}: a component can't contain an instance of itself`);
      }
    }
    if (n.type === "connector") {
      for (const end of [n.from, n.to]) {
        const t = ids.get(end.node);
        if (!t) errs.push(`${id}: endpoint ${end.node} doesn't exist`);
        else if (t.page !== page) errs.push(`${id}: endpoint ${end.node} is on another page`);
        else if (t.n.type === "connector") errs.push(`${id}: can't connect to a connector`);
      }
    }
  }
  const topFrames = new Set(design.pages.flatMap((p) => p.children.filter((n) => n.type === "frame").map((n) => n.id)));
  for (const l of design.links) {
    if (!ids.has(l.from)) errs.push(`link ${l.id}: from ${l.from} doesn't exist`);
    if (!topFrames.has(l.to)) errs.push(`link ${l.id}: to ${l.to} isn't a top-level frame`);
    if (l.trigger === "after-delay" && l.delay_ms === undefined) errs.push(`link ${l.id}: after-delay needs delay_ms`);
  }
  return errs;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  let bad = 0;
  for (const f of process.argv.slice(2)) {
    const raw = readFileSync(f, "utf8");
    const errs = validate(JSON.parse(raw), raw);
    console.log(errs.length ? `INVALID ${f}\n  ${errs.join("\n  ")}` : `valid ${f}`);
    bad += errs.length ? 1 : 0;
  }
  process.exit(bad ? 1 : 0);
}
