import type { Schemas } from "@/lib/api/client";

type CodeChange = Schemas["ChatCodeChange"];

export type DiffRow =
  | { kind: "hunk"; text: string }
  | { kind: "meta"; text: string }
  | { kind: "add" | "del" | "ctx"; text: string; oldNo?: number; newNo?: number };

const HUNK = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

/** Starting old/new line numbers from a `@@ -a,b +c,d @@` header, or null if it isn't one. */
export function parseHunkHeader(line: string): { oldStart: number; newStart: number } | null {
  const m = HUNK.exec(line);
  return m ? { oldStart: Number(m[1]), newStart: Number(m[3]) } : null;
}

/** Splits a unified diff (hunks only) into rows with old/new line numbers; text has the +/-/space prefix removed. */
export function parsePatch(patch: string): DiffRow[] {
  const rows: DiffRow[] = [];
  let oldNo = 0;
  let newNo = 0;
  const lines = patch.split("\n");
  if (lines.at(-1) === "") lines.pop();
  for (const line of lines) {
    const h = parseHunkHeader(line);
    if (h) {
      oldNo = h.oldStart;
      newNo = h.newStart;
      rows.push({ kind: "hunk", text: line });
    } else if (line.startsWith("\\")) rows.push({ kind: "meta", text: line });
    else if (line.startsWith("+")) rows.push({ kind: "add", text: line.slice(1), newNo: newNo++ });
    else if (line.startsWith("-")) rows.push({ kind: "del", text: line.slice(1), oldNo: oldNo++ });
    else rows.push({ kind: "ctx", text: line.slice(1), oldNo: oldNo++, newNo: newNo++ });
  }
  return rows;
}

export function changeTotals(changes: Pick<CodeChange, "additions" | "deletions">[]) {
  return changes.reduce((t, c) => ({ additions: t.additions + c.additions, deletions: t.deletions + c.deletions }), { additions: 0, deletions: 0 });
}

/** Files start expanded when the block opens only if there are few of them. */
export const filesStartOpen = (count: number) => count <= 3;

export const STATUS_BADGE: Record<CodeChange["status"], { letter: string; label: string }> = {
  added: { letter: "A", label: "Added" },
  modified: { letter: "M", label: "Modified" },
  deleted: { letter: "D", label: "Deleted" },
  renamed: { letter: "R", label: "Renamed" },
};

const BY_EXT: Record<string, string> = {
  ts: "typescript", mts: "typescript", cts: "typescript", tsx: "tsx", js: "javascript", mjs: "javascript", cjs: "javascript", jsx: "jsx",
  json: "json", go: "go", py: "python", rb: "ruby", rs: "rust", java: "java", kt: "kotlin", kts: "kotlin", swift: "swift",
  c: "c", h: "c", cc: "cpp", cpp: "cpp", hpp: "cpp", cs: "csharp", php: "php", sh: "shellscript", bash: "shellscript", zsh: "shellscript",
  yaml: "yaml", yml: "yaml", toml: "toml", md: "markdown", html: "html", css: "css", scss: "scss", sql: "sql", mk: "make",
};
const BY_NAME: Record<string, string> = { dockerfile: "docker", makefile: "make", gnumakefile: "make" };
const BY_FENCE: Record<string, string> = {
  ts: "typescript", typescript: "typescript", tsx: "tsx", js: "javascript", javascript: "javascript", jsx: "jsx", json: "json",
  go: "go", golang: "go", py: "python", python: "python", rb: "ruby", ruby: "ruby", rs: "rust", rust: "rust", java: "java",
  kt: "kotlin", kotlin: "kotlin", swift: "swift", c: "c", cpp: "cpp", "c++": "cpp", cs: "csharp", csharp: "csharp", php: "php",
  sh: "shellscript", bash: "shellscript", shell: "shellscript", zsh: "shellscript", yaml: "yaml", yml: "yaml", toml: "toml",
  md: "markdown", markdown: "markdown", html: "html", css: "css", scss: "scss", sql: "sql", dockerfile: "docker", make: "make", makefile: "make",
};

/** Shiki language id for a file path, or null (plain text). */
export function langFromPath(path: string): string | null {
  const name = (path.split("/").pop() ?? "").toLowerCase();
  if (BY_NAME[name]) return BY_NAME[name];
  const dot = name.lastIndexOf(".");
  return dot < 0 ? null : (BY_EXT[name.slice(dot + 1)] ?? null);
}

/** Shiki language id for a markdown fence info string (```ts), or null. */
export function langFromFence(info: string | undefined): string | null {
  const word = (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() ?? "";
  return BY_FENCE[word] ?? null;
}
