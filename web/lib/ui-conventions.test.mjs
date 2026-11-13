// App-wide UI conventions (DESIGN.md §2 and §6), checked over the source so a new screen
// can't quietly drift from them.
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import test from "node:test";

const web = new URL("..", import.meta.url).pathname;

function sources(dir) {
  return readdirSync(join(web, dir), { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return entry.name === "node_modules" ? [] : sources(path);
    return /\.tsx?$/.test(entry.name) && !/\.d\.ts$/.test(entry.name) ? [path] : [];
  });
}
const files = [...sources("app"), ...sources("components"), ...sources("lib")].map((path) => ({ path, text: readFileSync(join(web, path), "utf8") }));

function offenders(pattern, allow = () => false) {
  return files.flatMap(({ path, text }) => text.split("\n").flatMap((line, i) => pattern.test(line) && !allow(path, line) ? [`${relative(".", path)}:${i + 1}: ${line.trim().slice(0, 120)}`] : []));
}

test("font sizes come from the type scale, so Settings → Text size reaches them", () => {
  // text-[13px] can't follow --type-scale and Tailwind's text-xs… are cleared; use text-10 … text-32.
  assert.deepEqual(offenders(/(?<![\w-])text-\[\d+(\.\d+)?(px|rem)\]|(?<![\w-])text-(xs|sm|base|lg|xl|[2-9]xl)(?![\w-])/), []);
});

test("confirmations are in-app, never the browser's confirm() or alert()", () => {
  // confirmAction (components/ui/confirm.tsx) for what can't be undone, undoable (lib/toast.ts) for the rest.
  assert.deepEqual(offenders(/(?<![\w.])(window\.)?(confirm|alert)\(/), []);
});

test("loading is a skeleton or a Spinner, never the word on screen or a hand-rolled spinner", () => {
  // components/ui/loading.tsx: LoadingLines / LoadingText for sections and values, Spinner in buttons.
  assert.deepEqual(offenders(/Loading(…|\.\.\.)|animate-spin/, (path) => path === "components/ui/loading.tsx"), []);
});
