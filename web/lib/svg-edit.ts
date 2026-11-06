/**
 * Pure edit operations for the visual mockup editor. They mutate a parsed SVG
 * document in place (callers parse a fresh doc per operation, mutate, then
 * `serialize`). Paths are child-index paths from the document element, the same
 * ones svg-sanitize emits ("" = root, "0.3.1" = a descendant).
 */

export type ShapeKind = "rect" | "ellipse" | "line" | "text";

const NS = "http://www.w3.org/2000/svg";
const WIRE_FILL = "#F3F4F6";
const WIRE_STROKE = "#9CA3AF";
const TEXT_FILL = "#111827";

/** Parse source to an inert SVG document, or null when it isn't well-formed SVG. */
export function parseSvg(source: string): Document | null {
  if (typeof DOMParser === "undefined") return null;
  const doc = new DOMParser().parseFromString(source, "image/svg+xml");
  if (doc.getElementsByTagName("parsererror").length) return null;
  const root = doc.documentElement;
  return root && root.localName === "svg" && root.namespaceURI === NS ? doc : null;
}

export function serialize(doc: Document): string {
  return new XMLSerializer().serializeToString(doc);
}

export function findByPath(doc: Document, path: string): Element | null {
  let n: Node | null = doc.documentElement;
  if (path === "") return doc.documentElement;
  for (const seg of path.split(".")) {
    const i = Number(seg);
    n = n && Number.isInteger(i) ? (n.childNodes[i] ?? null) : null;
    if (!n) return null;
  }
  return n && n.nodeType === 1 ? (n as Element) : null;
}

export function pathOf(doc: Document, el: Element): string | null {
  const segs: number[] = [];
  let n: Node = el;
  while (n !== doc.documentElement) {
    const p: Node | null = n.parentNode;
    if (!p || p === doc) return null;
    segs.unshift(Array.prototype.indexOf.call(p.childNodes, n));
    n = p;
  }
  return segs.join(".");
}

const round = (n: number) => Math.round(n * 2) / 2;
const fmt = (n: number) => String(+n.toFixed(2) || 0);
const TRANSLATE = /^\s*translate\(\s*([-+]?[\d.]+(?:e[-+]?\d+)?)(?:[\s,]+([-+]?[\d.]+(?:e[-+]?\d+)?))?\s*\)\s*/i;

/** Move an element by merging into / prepending a translate() in its transform. Delta rounds to 0.5. */
export function translate(el: Element, dx: number, dy: number): void {
  dx = round(dx);
  dy = round(dy);
  if (!dx && !dy) return;
  const cur = el.getAttribute("transform") ?? "";
  const m = TRANSLATE.exec(cur);
  let tx = dx;
  let ty = dy;
  let rest = cur.trim();
  if (m) {
    tx += Number(m[1]);
    ty += Number(m[2] ?? 0);
    rest = cur.slice(m[0].length).trim();
  }
  const next = tx || ty ? `translate(${fmt(tx)} ${fmt(ty)})${rest ? " " + rest : ""}` : rest;
  if (next) el.setAttribute("transform", next);
  else el.removeAttribute("transform");
}

function childElements(el: Element, name: string): Element[] {
  return Array.from(el.childNodes).filter((n): n is Element => n.nodeType === 1 && (n as Element).localName === name);
}

/** The element that actually holds a text element's characters (its first tspan, if any). */
function textHolder(el: Element): Element {
  return childElements(el, "tspan")[0] ?? el;
}

export function getText(el: Element): string {
  return textHolder(el).textContent ?? "";
}

export function setText(el: Element, text: string): void {
  const holder = textHolder(el);
  while (holder.firstChild) holder.removeChild(holder.firstChild);
  holder.appendChild(el.ownerDocument.createTextNode(text));
}

/** Set an attribute; null or "" removes it. */
export function setAttr(el: Element, name: string, value: string | null): void {
  if (value == null || value === "") el.removeAttribute(name);
  else el.setAttribute(name, value);
}

const isBlank = (n: Node | null): n is Node => !!n && n.nodeType === 3 && !(n.nodeValue ?? "").trim();

/** Remove an element (never the root) along with its leading indentation. */
export function remove(el: Element): boolean {
  const p = el.parentNode;
  if (!p || p.nodeType !== 1) return false;
  if (isBlank(el.previousSibling)) p.removeChild(el.previousSibling);
  p.removeChild(el);
  return true;
}

function stripIds(el: Element) {
  el.removeAttribute("id");
  for (const c of Array.from(el.childNodes)) if (c.nodeType === 1) stripIds(c as Element);
}

/** Insert a copy right after `el`, offset by 16,16. Ids are stripped from the copy. */
export function duplicate(el: Element): Element | null {
  const p = el.parentNode;
  if (!p || p.nodeType !== 1) return null;
  const copy = el.cloneNode(true) as Element;
  stripIds(copy);
  translate(copy, 16, 16);
  const ws = isBlank(el.previousSibling) ? el.previousSibling.cloneNode(false) : null;
  const before = el.nextSibling;
  if (ws) p.insertBefore(ws, before);
  p.insertBefore(copy, before);
  return copy;
}

function sibling(el: Element, dir: 1 | -1): Element | null {
  let n: Node | null = dir > 0 ? el.nextSibling : el.previousSibling;
  while (n && n.nodeType !== 1) n = dir > 0 ? n.nextSibling : n.previousSibling;
  return n as Element | null;
}

/** Move one step later in z-order (drawn on top of its next sibling). */
export function bringForward(el: Element): boolean {
  const next = sibling(el, 1);
  if (!next || !el.parentNode) return false;
  el.parentNode.insertBefore(el, next.nextSibling);
  return true;
}

export function sendBackward(el: Element): boolean {
  const prev = sibling(el, -1);
  if (!prev || !el.parentNode) return false;
  el.parentNode.insertBefore(el, prev);
  return true;
}

/** Append a wireframe-styled shape centred on `at` (viewBox units); returns it. */
export function addShape(doc: Document, kind: ShapeKind, at: { x: number; y: number }): Element {
  const root = doc.documentElement;
  const el = doc.createElementNS(NS, kind);
  const set = (k: string, v: string | number) => el.setAttribute(k, String(v));
  const x = Math.round(at.x);
  const y = Math.round(at.y);
  if (kind === "rect") {
    set("x", x - 80); set("y", y - 50); set("width", 160); set("height", 100); set("rx", 6);
    set("fill", WIRE_FILL); set("stroke", WIRE_STROKE);
  } else if (kind === "ellipse") {
    set("cx", x); set("cy", y); set("rx", 80); set("ry", 50);
    set("fill", WIRE_FILL); set("stroke", WIRE_STROKE);
  } else if (kind === "line") {
    set("x1", x - 80); set("y1", y); set("x2", x + 80); set("y2", y);
    set("stroke", WIRE_STROKE); set("stroke-width", 2);
  } else {
    set("x", x - 24); set("y", y); set("fill", TEXT_FILL); set("font-size", 16); set("font-family", "sans-serif");
    el.appendChild(doc.createTextNode("Text"));
  }
  // Keep the trailing newline before </svg> last.
  const last = root.lastChild;
  const tail = isBlank(last) ? last : null;
  root.insertBefore(doc.createTextNode("\n  "), tail);
  root.insertBefore(el, tail);
  return el;
}
