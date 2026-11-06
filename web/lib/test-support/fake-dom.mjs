// A tiny XML parser + DOM subset for node --test (no jsdom in this repo).
// Implements only what svg-sanitize / svg-edit touch.
const SVG = "http://www.w3.org/2000/svg";
const XLINK = "http://www.w3.org/1999/xlink";

class FNode {
  parentNode = null;
  childNodes = [];
  get firstChild() { return this.childNodes[0] ?? null; }
  get lastChild() { return this.childNodes[this.childNodes.length - 1] ?? null; }
  get nextSibling() { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null; }
  get previousSibling() { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) - 1] ?? null; }
  _detach(n) { n.parentNode?.removeChild(n); }
  appendChild(n) { return this.insertBefore(n, null); }
  insertBefore(n, ref) {
    this._detach(n);
    n.parentNode = this;
    const i = ref ? this.childNodes.indexOf(ref) : this.childNodes.length;
    this.childNodes.splice(i, 0, n);
    return n;
  }
  removeChild(n) {
    this.childNodes.splice(this.childNodes.indexOf(n), 1);
    n.parentNode = null;
    return n;
  }
  get textContent() { return this.childNodes.map((c) => (c.nodeType === 3 ? c.nodeValue : c.textContent)).join(""); }
}

class FText extends FNode {
  nodeType = 3;
  constructor(v, doc) { super(); this.nodeValue = v; this.ownerDocument = doc; }
  cloneNode() { return new FText(this.nodeValue, this.ownerDocument); }
}

class FComment extends FNode {
  nodeType = 8;
  constructor(v) { super(); this.nodeValue = v; }
}

class FElement extends FNode {
  nodeType = 1;
  constructor(ns, localName, doc) { super(); this.namespaceURI = ns; this.localName = localName; this.ownerDocument = doc; this.attributes = []; }
  getAttribute(n) { return this.attributes.find((a) => a.name === n)?.value ?? null; }
  setAttribute(n, v) {
    const a = this.attributes.find((x) => x.name === n);
    if (a) a.value = String(v); else this.attributes.push({ name: n, localName: n, namespaceURI: null, value: String(v) });
  }
  removeAttribute(n) { this.attributes = this.attributes.filter((a) => a.name !== n); }
  cloneNode(deep) {
    const c = new FElement(this.namespaceURI, this.localName, this.ownerDocument);
    c.attributes = this.attributes.map((a) => ({ ...a }));
    if (deep) for (const k of this.childNodes) c.appendChild(k.cloneNode(true));
    return c;
  }
}

export class FDoc extends FNode {
  nodeType = 9;
  get documentElement() { return this.childNodes.find((c) => c.nodeType === 1) ?? null; }
  createElementNS(ns, n) { return new FElement(ns, n, this); }
  createTextNode(v) { return new FText(v, this); }
}

/** Parse a small XML string (no DOCTYPE/entities beyond &amp; &lt; &gt; &quot;). */
export function parse(src) {
  const doc = new FDoc();
  const stack = [{ node: doc, ns: { "": null } }];
  const re = /<!--([\s\S]*?)-->|<!\[CDATA\[([\s\S]*?)\]\]>|<(\/?)([\w:.-]+)((?:\s+[\w:.-]+\s*=\s*(?:"[^"]*"|'[^']*'))*)\s*(\/?)>|([^<]+)/g;
  const unesc = (s) => s.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"').replace(/&amp;/g, "&");
  let m;
  while ((m = re.exec(src))) {
    const top = stack[stack.length - 1];
    if (m[1] !== undefined) top.node.appendChild(new FComment(m[1]));
    else if (m[2] !== undefined) { const t = new FText(m[2], doc); t.nodeType = 4; top.node.appendChild(t); }
    else if (m[7] !== undefined) top.node.appendChild(new FText(unesc(m[7]), doc));
    else if (m[3]) stack.pop();
    else {
      const raw = [...m[5].matchAll(/([\w:.-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')/g)].map((a) => [a[1], unesc(a[2] ?? a[3])]);
      const ns = { ...top.ns };
      for (const [k, v] of raw) {
        if (k === "xmlns") ns[""] = v;
        else if (k.startsWith("xmlns:")) ns[k.slice(6)] = v;
      }
      if (ns.xlink === undefined) ns.xlink = XLINK;
      const [p, l] = m[4].includes(":") ? m[4].split(":") : ["", m[4]];
      const el = new FElement(ns[p] ?? null, l, doc);
      for (const [k, v] of raw) {
        let a;
        if (k === "xmlns" || k.startsWith("xmlns:")) a = { name: k, localName: k.replace("xmlns:", ""), namespaceURI: "http://www.w3.org/2000/xmlns/", value: v };
        else if (k.includes(":")) { const [ap, al] = k.split(":"); a = { name: k, localName: al, namespaceURI: ns[ap] ?? null, value: v }; }
        else a = { name: k, localName: k, namespaceURI: null, value: v };
        el.attributes.push(a);
      }
      top.node.appendChild(el);
      if (!m[6]) stack.push({ node: el, ns });
    }
  }
  return doc;
}

export function ser(n) {
  if (n.nodeType === 3 || n.nodeType === 4) return n.nodeValue;
  if (n.nodeType === 8) return `<!--${n.nodeValue}-->`;
  if (n.nodeType === 9) return n.childNodes.map(ser).join("");
  const attrs = n.attributes.filter((a) => !a.name.startsWith("xmlns")).map((a) => ` ${a.name}="${a.value}"`).join("");
  return n.childNodes.length ? `<${n.localName}${attrs}>${n.childNodes.map(ser).join("")}</${n.localName}>` : `<${n.localName}${attrs}/>`;
}

export { SVG, XLINK };
