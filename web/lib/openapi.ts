// The Playground's API tab: an app's OpenAPI document, as operations to try.
// Untrusted input (the agent can write the document), so it is only read as
// data: descriptions are shown as plain text. Kept free of React so
// `npm run test:unit` covers it.

/* eslint-disable @typescript-eslint/no-explicit-any */
type Json = any;

export type Param = { name: string; in: "path" | "query" | "header" | "cookie"; required: boolean; description: string; schema: Json };

export type Operation = {
  key: string; // "PUT /runner/test-environments"
  method: string; // upper case
  path: string;
  operationId: string;
  summary: string;
  description: string;
  tag: string;
  params: Param[];
  body?: { contentType: string; required: boolean; schema: Json };
};

export type ApiDoc = { title: string; base: string; groups: { tag: string; ops: Operation[] }[]; count: number };

const METHODS = ["get", "put", "post", "delete", "patch", "head", "options"];

/** What a local $ref points at ("#/components/schemas/X"), or undefined. */
export function resolveRef(doc: Json, ref: string): Json {
  if (!ref.startsWith("#/")) return undefined;
  let node = doc;
  for (const raw of ref.slice(2).split("/")) {
    const part = raw.replace(/~1/g, "/").replace(/~0/g, "~");
    node = node?.[part];
    if (node === undefined) return undefined;
  }
  return node;
}

function deref(doc: Json, node: Json, depth = 0): Json {
  while (node && typeof node.$ref === "string" && depth++ < 20) node = resolveRef(doc, node.$ref);
  return node;
}

const str = (v: unknown) => (typeof v === "string" ? v : "");

/**
 * The path every operation is under: the first server's path. A server with
 * a host is read for its path only, because the playground is the host.
 */
export function basePath(doc: Json): string {
  const url = str(doc?.servers?.[0]?.url);
  if (!url) return "";
  let path = url;
  const abs = /^(?:[a-z][a-z0-9+.-]*|\{[^}]*\}):\/\/[^/]*/i.exec(url);
  if (abs) path = url.slice(abs[0].length);
  path = path.replace(/\{[^}]*\}/g, "").replace(/\/+$/, "");
  return path && !path.startsWith("/") ? "/" + path : path;
}

/** The document's operations, grouped by their first tag (in the document's tag order). */
export function parseOpenAPI(doc: Json): ApiDoc {
  if (!doc || typeof doc !== "object" || typeof doc.paths !== "object") throw new Error("Not an OpenAPI document: it has no paths.");
  const groups = new Map<string, Operation[]>();
  for (const t of Array.isArray(doc.tags) ? doc.tags : []) if (typeof t?.name === "string") groups.set(t.name, []);
  let count = 0;
  for (const [path, rawItem] of Object.entries<Json>(doc.paths)) {
    const item = deref(doc, rawItem);
    if (!item || typeof item !== "object") continue;
    const shared: Json[] = Array.isArray(item.parameters) ? item.parameters : [];
    for (const m of METHODS) {
      const op = item[m];
      if (!op || typeof op !== "object") continue;
      const params = new Map<string, Param>();
      for (const raw of [...shared, ...(Array.isArray(op.parameters) ? op.parameters : [])]) {
        const p = deref(doc, raw);
        if (!p || typeof p.name !== "string" || !["path", "query", "header", "cookie"].includes(p.in)) continue;
        params.set(`${p.in}:${p.name}`, { name: p.name, in: p.in, required: p.in === "path" || !!p.required, description: str(p.description), schema: p.schema ?? {} });
      }
      let body: Operation["body"];
      const rb = deref(doc, op.requestBody);
      if (rb?.content && typeof rb.content === "object") {
        const types = Object.keys(rb.content);
        const contentType = types.find((t) => /json/i.test(t)) ?? types[0];
        if (contentType) body = { contentType, required: !!rb.required, schema: rb.content[contentType]?.schema ?? {} };
      }
      const tag = Array.isArray(op.tags) && typeof op.tags[0] === "string" ? op.tags[0] : "other";
      const operation: Operation = {
        key: `${m.toUpperCase()} ${path}`,
        method: m.toUpperCase(),
        path,
        operationId: str(op.operationId),
        summary: str(op.summary),
        description: str(op.description),
        tag,
        params: [...params.values()],
        body,
      };
      if (!groups.has(tag)) groups.set(tag, []);
      groups.get(tag)!.push(operation);
      count++;
    }
  }
  return {
    title: str(doc.info?.title),
    base: basePath(doc),
    groups: [...groups].filter(([, ops]) => ops.length > 0).map(([tag, ops]) => ({ tag, ops })),
    count,
  };
}

/** Operations whose method, path, id, summary or tag contain every word of the filter. */
export function filterOps(doc: ApiDoc, filter: string): ApiDoc["groups"] {
  const words = filter.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return doc.groups;
  return doc.groups
    .map((g) => ({ tag: g.tag, ops: g.ops.filter((o) => words.every((w) => `${o.key} ${o.operationId} ${o.summary} ${g.tag}`.toLowerCase().includes(w))) }))
    .filter((g) => g.ops.length > 0);
}

/** A value shaped like the schema: its example or default, else one built from its type. */
export function exampleFor(doc: Json, schema: Json, depth = 0): Json {
  const s = deref(doc, schema);
  if (!s || typeof s !== "object" || depth > 6) return null;
  if (s.example !== undefined) return s.example;
  if (s.default !== undefined) return s.default;
  if (Array.isArray(s.enum) && s.enum.length) return s.enum[0];
  if (Array.isArray(s.allOf)) return s.allOf.reduce((acc: Json, part: Json) => {
    const v = exampleFor(doc, part, depth + 1);
    return v && typeof v === "object" && !Array.isArray(v) ? { ...acc, ...v } : acc;
  }, {});
  const alt = s.oneOf ?? s.anyOf;
  if (Array.isArray(alt) && alt.length) return exampleFor(doc, alt[0], depth + 1);
  switch (typeOf(s)) {
    case "object": {
      const out: Record<string, Json> = {};
      const required: string[] = Array.isArray(s.required) ? s.required : [];
      for (const [k, v] of Object.entries<Json>(s.properties ?? {})) {
        // Required fields, and all of them on a small object.
        if (required.includes(k) || Object.keys(s.properties).length <= 4) out[k] = exampleFor(doc, v, depth + 1);
      }
      return out;
    }
    case "array":
      return [exampleFor(doc, s.items, depth + 1)];
    case "integer":
    case "number":
      return typeof s.minimum === "number" ? s.minimum : 0;
    case "boolean":
      return false;
    case "string":
      return s.format === "uuid" ? "00000000-0000-0000-0000-000000000000" : s.format === "date-time" ? new Date(0).toISOString() : "string";
  }
  return null;
}

function typeOf(s: Json): string {
  if (typeof s.type === "string") return s.type;
  if (Array.isArray(s.type)) return s.type.find((t: string) => t !== "null") ?? "null";
  if (s.properties) return "object";
  if (s.items) return "array";
  return "";
}

export type Warning = { path: string; message: string };

/** Where a value doesn't match the schema. A warning only: sending bad input is part of testing. */
export function validate(doc: Json, schema: Json, value: Json, path = "", depth = 0): Warning[] {
  const s = deref(doc, schema);
  if (!s || typeof s !== "object" || depth > 12) return [];
  const at = path || "body";
  if (value === null) return s.nullable || (Array.isArray(s.type) && s.type.includes("null")) || !typeOf(s) ? [] : [{ path: at, message: `expected ${typeOf(s)}, got null` }];
  if (Array.isArray(s.allOf)) return s.allOf.flatMap((part: Json) => validate(doc, part, value, path, depth + 1));
  const alt = s.oneOf ?? s.anyOf;
  if (Array.isArray(alt)) return alt.some((part: Json) => validate(doc, part, value, path, depth + 1).length === 0) ? [] : [{ path: at, message: "matches none of the allowed shapes" }];
  if (Array.isArray(s.enum) && !s.enum.includes(value)) return [{ path: at, message: `must be one of ${s.enum.map((e: Json) => JSON.stringify(e)).join(", ")}` }];
  const t = typeOf(s);
  const actual = Array.isArray(value) ? "array" : typeof value;
  const ok =
    !t ||
    (t === "integer" ? Number.isInteger(value) : t === "number" ? typeof value === "number" : t === "array" ? Array.isArray(value) : t === "object" ? actual === "object" && !Array.isArray(value) : actual === t);
  if (!ok) return [{ path: at, message: `expected ${t}, got ${actual}` }];
  const out: Warning[] = [];
  if (t === "object") {
    for (const k of Array.isArray(s.required) ? s.required : []) if (!(k in value)) out.push({ path: `${path ? path + "." : ""}${k}`, message: "is required" });
    for (const [k, v] of Object.entries<Json>(value)) {
      const prop = s.properties?.[k];
      if (prop) out.push(...validate(doc, prop, v, `${path ? path + "." : ""}${k}`, depth + 1));
    }
  }
  if (t === "array") value.forEach((v: Json, i: number) => out.push(...validate(doc, s.items, v, `${at}[${i}]`, depth + 1)));
  return out;
}

/** The request URL path: base, path parameters filled (encoded), query appended. */
export function requestPath(base: string, op: Operation, values: Record<string, string>): string {
  const path = op.path.replace(/\{([^}]+)\}/g, (_, name: string) => encodeURIComponent(values[`path:${name}`] ?? ""));
  const q = new URLSearchParams();
  for (const p of op.params) if (p.in === "query" && values[`query:${p.name}`]) q.set(p.name, values[`query:${p.name}`]);
  const qs = q.toString();
  return `${base}${path}${qs ? `?${qs}` : ""}`;
}

/** The same request for a terminal. Cookies are left out: they're the browser's. */
export function curlFor(method: string, url: string, headers: Record<string, string>, body?: string): string {
  const quote = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;
  const parts = ["curl", "-X", method, quote(url)];
  for (const [k, v] of Object.entries(headers)) if (v) parts.push("-H", quote(`${k}: ${v}`));
  if (body) parts.push("--data", quote(body));
  return parts.join(" ");
}
