import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { THEMES, THEME_ORDER, THEME_COLORS, contrastIssues, contrastRatio } from "./slides/schema.ts";

test("the built-in themes match the server's, all ten in menu order", () => {
  const go = readFileSync(new URL("../../server/internal/deck/templates.go", import.meta.url), "utf8");
  assert.ok(THEME_ORDER.length >= 10);
  assert.deepEqual(Object.keys(THEMES), THEME_ORDER);
  for (const k of THEME_ORDER) {
    const t = THEMES[k];
    const m = go.match(new RegExp(`"${k}": \\{Name: "([^"]+)", Fonts: map\\[string\\]string\\{"heading": "(\\w+)", "body": "(\\w+)"\\},\\s*Colors: map\\[string\\]string\\{([^}]*)\\}`));
    assert.ok(m, `${k} missing from templates.go`);
    assert.equal(t.name, m[1]);
    assert.deepEqual(t.fonts, { heading: m[2], body: m[3] });
    const colors = Object.fromEntries([...m[4].matchAll(/"(\w+)": "(#[0-9A-F]{6})"/g)].map((x) => [x[1], x[2]]));
    assert.deepEqual(t.colors, colors, k);
    assert.deepEqual(Object.keys(t.colors).sort(), [...THEME_COLORS].sort());
  }
});

test("every built-in theme meets WCAG AA for text, heading and muted", () => {
  for (const k of THEME_ORDER) assert.deepEqual(contrastIssues(THEMES[k]), [], k);
});

test("contrast ratios and warnings", () => {
  assert.ok(Math.abs(contrastRatio("#000000", "#FFFFFF") - 21) < 0.01);
  const low = { ...THEMES.light, colors: { ...THEMES.light.colors, muted: "#CCCCCC" } };
  assert.deepEqual(contrastIssues(low).map((i) => `${i.fg}/${i.bg}`), ["muted/background", "muted/surface"]);
});
