// Mermaid flowchart -> design nodes + connectors with automatic layout.
//
// Mermaid's own parser needs a DOM (DOMPurify), so it can't run in the Node
// design worker; this is a small parser for the flowchart subset we support,
// shared by the browser and the worker. ELK (elkjs, layered, orthogonal edge
// routing) places the nodes and routes the connectors.
//
// Supported: flowchart|graph TD/TB/BT/LR/RL; node shapes [] () ([]) (()) {}
// {{}} [()] >]; quoted labels; edges --> --- -.-> -.- ==> with |label| or
// -- label --> / -. label .-> / == label ==>; chains (a --> b --> c); & lists;
// subgraph id [title] ... end (nested); %% comments; ; separators.
// Ignored with a warning: classDef, class, style, linkStyle, click.
// Anything else is refused with the line number and the text that failed.
import ELK from "elkjs/lib/elk.bundled.js";

export class MermaidError extends Error {}

const SHAPES = [
  // [open, close, shape kind]
  ["([", "])", "stadium"],
  ["((", "))", "circle"],
  ["[(", ")]", "cylinder"],
  ["{{", "}}", "hexagon"],
  ["[", "]", "rect"],
  ["(", ")", "round"],
  ["{", "}", "diamond"],
  [">", "]", "flag"],
];
const ID = /^[A-Za-z_]\w*(?:-[A-Za-z0-9_]+)*/; // a hyphen only inside an id, so "a-->b" is an edge
// Edge operators, longest first. Captures an inline label for the "-- text -->" forms.
const EDGES = [
  { re: /^-\.\s*([^.|]+?)\s*\.->/, dash: true, arrow: true, thick: false },
  { re: /^==\s*([^=|]+?)\s*==>/, dash: false, arrow: true, thick: true },
  { re: /^--\s*([^-|>]+?)\s*-->/, dash: false, arrow: true, thick: false },
  { re: /^-\.->/, dash: true, arrow: true, thick: false },
  { re: /^-\.-/, dash: true, arrow: false, thick: false },
  { re: /^==>/, dash: false, arrow: true, thick: true },
  { re: /^-->/, dash: false, arrow: true, thick: false },
  { re: /^---/, dash: false, arrow: false, thick: false },
];
const IGNORED = /^(classDef|class|style|linkStyle|click)\b/;

export function parseFlowchart(src) {
  const lines = src.split(/\r?\n/);
  const nodes = new Map(); // id -> {id, label, shape, parent}
  const edges = [];
  const groups = []; // {id, title, parent}
  const warnings = [];
  const stack = [];
  let direction = null;
  const fail = (ln, msg, text) => {
    throw new MermaidError(`line ${ln + 1}: ${msg}${text ? `: "${text.trim().slice(0, 60)}"` : ""}`);
  };
  for (let ln = 0; ln < lines.length; ln++) {
    const statements = lines[ln].replace(/%%.*$/, "").split(";");
    for (let s of statements) {
      s = s.trim();
      if (!s) continue;
      if (!direction) {
        const m = s.match(/^(flowchart|graph)\s+(TD|TB|BT|LR|RL)\s*$/);
        if (!m) fail(ln, "expected 'flowchart' or 'graph' and a direction (TD, TB, BT, LR or RL); only flowcharts can be imported", s);
        direction = m[2] === "TD" ? "TB" : m[2];
        continue;
      }
      if (IGNORED.test(s)) {
        warnings.push(`line ${ln + 1}: ${s.split(/\s/)[0]} is ignored (styling isn't imported)`);
        continue;
      }
      const sg = s.match(/^subgraph\s+([A-Za-z_][\w-]*)\s*(?:\[\s*"?([^"\]]*)"?\s*\])?\s*$/) || s.match(/^subgraph\s+"?([^"]+?)"?\s*$/);
      if (sg) {
        const id = sg[1].replace(/\s+/g, "_");
        groups.push({ id, title: sg[2] ?? sg[1], parent: stack.at(-1) ?? null });
        stack.push(id);
        continue;
      }
      if (s === "end") {
        if (!stack.length) fail(ln, "'end' without a subgraph");
        stack.pop();
        continue;
      }
      if (/^direction\s+/.test(s)) {
        warnings.push(`line ${ln + 1}: direction inside a subgraph is ignored`);
        continue;
      }
      parseStatement(s, ln);
    }
  }
  if (!direction) throw new MermaidError("line 1: the text is empty; expected 'flowchart LR' or similar");
  if (stack.length) throw new MermaidError(`subgraph ${stack.at(-1)} has no 'end'`);
  return { direction, nodes: [...nodes.values()], edges, groups, warnings };

  function node(rest, ln) {
    const m = rest.match(ID);
    if (!m) fail(ln, "expected a node id", rest);
    const id = m[0];
    rest = rest.slice(id.length);
    let label, shape;
    for (const [open, close, kind] of SHAPES) {
      if (!rest.startsWith(open)) continue;
      const end = rest.indexOf(close, open.length);
      if (end < 0) fail(ln, `node ${id} opens ${open} but never closes it with ${close}`, rest);
      label = rest.slice(open.length, end).trim().replace(/^"(.*)"$/, "$1");
      shape = kind;
      rest = rest.slice(end + close.length);
      break;
    }
    const existing = nodes.get(id);
    if (!existing) nodes.set(id, { id, label: label ?? id, shape: shape ?? "rect", parent: stack.at(-1) ?? null });
    else if (label !== undefined) Object.assign(existing, { label, shape });
    return { id, rest: rest.trimStart() };
  }
  function nodeList(rest, ln) {
    const ids = [];
    for (;;) {
      const r = node(rest, ln);
      ids.push(r.id);
      rest = r.rest;
      if (!rest.startsWith("&")) return { ids, rest };
      rest = rest.slice(1).trimStart();
    }
  }
  function parseStatement(s, ln) {
    let { ids: from, rest } = nodeList(s, ln);
    while (rest) {
      const op = EDGES.find((e) => e.re.test(rest));
      if (!op) fail(ln, "expected an edge (-->, ---, -.->, ==>) or the end of the line", rest);
      const m = rest.match(op.re);
      let label = m[1]?.trim();
      rest = rest.slice(m[0].length).trimStart();
      const pipe = rest.match(/^\|([^|]*)\|/);
      if (pipe) {
        label = pipe[1].trim().replace(/^"(.*)"$/, "$1");
        rest = rest.slice(pipe[0].length).trimStart();
      }
      if (!rest) fail(ln, "the edge has no target node", s);
      const r = nodeList(rest, ln);
      for (const a of from) for (const b of r.ids) edges.push({ from: a, to: b, label, dash: op.dash, arrow: op.arrow, thick: op.thick });
      from = r.ids;
      rest = r.rest;
    }
  }
}

const SHAPE_MAP = {
  rect: { shape: "roundRect", radius: 6 },
  round: { shape: "roundRect", radius: 14 },
  stadium: { shape: "roundRect", radius: 999 },
  circle: { shape: "ellipse" },
  diamond: { shape: "diamond" },
  hexagon: { shape: "hexagon" },
  cylinder: { shape: "roundRect", radius: 6, warn: "cylinder drawn as a rectangle" },
  flag: { shape: "roundRect", radius: 0, warn: "asymmetric shape drawn as a rectangle" },
};
const FONT = 15;
const PAD = { x: 20, y: 14 };

/**
 * Lays the parsed chart out with ELK and returns one section node holding
 * shapes, nested sections (subgraphs) and connectors with routed points, in
 * schema 1, at (x, y) on the page. ids: a function minting node ids.
 */
export async function importFlowchart(src, { measure, x = 0, y = 0, ids }) {
  const chart = parseFlowchart(src);
  const warnings = [...chart.warnings];
  const nid = new Map(chart.nodes.map((n) => [n.id, ids()]));
  const gid = new Map(chart.groups.map((g) => [g.id, ids()]));
  const sizeOf = (n) => {
    const w = measure(n.label, 500, false, FONT) + 2 * PAD.x;
    const h = FONT * 1.2 + 2 * PAD.y;
    if (n.shape === "diamond") return { w: Math.max(w * 1.5, 120), h: Math.max(h * 1.6, 72) };
    if (n.shape === "circle") return { w: Math.max(w, h), h: Math.max(w, h) };
    if (n.shape === "hexagon") return { w: w + 24, h };
    return { w: Math.max(w, 96), h };
  };
  const elkNode = (n) => ({ id: n.id, ...((s) => ({ width: s.w, height: s.h }))(sizeOf(n)) });
  const groupChildren = (gidName) => [
    ...chart.groups.filter((g) => g.parent === gidName).map((g) => ({
      id: `g:${g.id}`,
      layoutOptions: { "elk.padding": "[top=44,left=20,bottom=20,right=20]" },
      children: groupChildren(g.id),
    })),
    ...chart.nodes.filter((n) => n.parent === gidName).map(elkNode),
  ];
  const graph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": { LR: "RIGHT", RL: "LEFT", TB: "DOWN", BT: "UP" }[chart.direction],
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.hierarchyHandling": "INCLUDE_CHILDREN",
      "elk.layered.spacing.nodeNodeBetweenLayers": "56",
      "elk.spacing.nodeNode": "36",
      "elk.spacing.edgeNode": "20",
      "elk.spacing.edgeEdge": "14",
      "elk.layered.nodePlacement.strategy": "BRANDES_KOEPF",
      "elk.padding": "[top=56,left=32,bottom=32,right=32]",
    },
    children: groupChildren(null),
    edges: chart.edges.map((e, i) => ({
      id: `e${i}`,
      sources: [e.from],
      targets: [e.to],
      labels: e.label ? [{ text: e.label, width: measure(e.label, 400, false, 12) + 12, height: 20 }] : [],
    })),
  };
  const laid = await new ELK().layout(graph);

  // ELK coordinates are relative to each node's parent. An edge's points are
  // relative to its container (the lowest common ancestor ELK picked), so
  // shift them into the import section's (root) coordinates.
  const offset = new Map([["root", { x: 0, y: 0 }]]);
  const walkOff = (c, ox, oy) => {
    for (const k of c.children ?? []) {
      offset.set(k.id, { x: ox + k.x, y: oy + k.y });
      walkOff(k, ox + k.x, oy + k.y);
    }
  };
  walkOff(laid, 0, 0);
  const toNodes = (elkChildren) =>
    elkChildren.map((c) => {
      if (c.id.startsWith("g:")) {
        const g = chart.groups.find((x) => x.id === c.id.slice(2));
        return { id: gid.get(g.id), type: "section", name: g.title, x: c.x, y: c.y, w: c.width, h: c.height, fill: "$surface-raised", children: toNodes(c.children ?? []) };
      }
      const n = chart.nodes.find((x) => x.id === c.id);
      const m = SHAPE_MAP[n.shape];
      if (m.warn) warnings.push(`${n.id}: ${m.warn}`);
      return {
        id: nid.get(n.id), type: "shape", shape: m.shape, name: n.label, x: c.x, y: c.y, w: c.width, h: c.height,
        ...(m.radius !== undefined ? { radius: Math.min(m.radius, c.height / 2) } : {}),
        fill: n.shape === "diamond" ? "$accent-soft" : "$surface", stroke: n.shape === "diamond" ? "$accent" : "$border-strong", stroke_width: 1.5,
        text: { valign: "middle", paragraphs: [{ align: "center", runs: [{ text: n.label, size: FONT, weight: 500, color: "$text" }] }] },
      };
    });
  const connectors = laid.edges.map((e, i) => {
    const src = chart.edges[i];
    const sec = e.sections?.[0];
    if (!sec) throw new MermaidError(`edge ${src.from} -> ${src.to} could not be routed`);
    const o = offset.get(e.container ?? "root");
    const pts = [sec.startPoint, ...(sec.bendPoints ?? []), sec.endPoint].map((p) => [Math.round((o.x + p.x) * 100) / 100, Math.round((o.y + p.y) * 100) / 100]);
    return {
      id: ids(), type: "connector", from: { node: nid.get(src.from), anchor: "auto" }, to: { node: nid.get(src.to), anchor: "auto" },
      route: "orthogonal", stroke: src.thick ? "$text-2" : "$text-secondary", stroke_width: src.thick ? 3 : 1.5,
      ...(src.dash ? { dash: "dash" } : {}), end_arrow: src.arrow ? "arrow" : "none", ...(src.label ? { label: src.label } : {}), points: pts,
      _container: e.container,
    };
  });
  const section = {
    id: ids(), type: "section", name: "Imported flowchart", x, y, w: laid.width, h: laid.height, fill: "$surface",
    children: [...toNodes(laid.children), ...connectors.map(({ _container, ...c }) => c)],
  };
  return { section, warnings, containers: [...new Set(connectors.map((c) => c._container))] };
}
