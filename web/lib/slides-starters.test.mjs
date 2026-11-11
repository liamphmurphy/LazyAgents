import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { layoutSlide } from "./slides/layout.ts";
import { LAYOUTS, THEMES, THEME_ORDER, placeholders } from "./slides/schema.ts";

// Starter decks (LAZYAGENTS-123) are fixtures the server embeds and validates
// (server/internal/deck/starters_test.go). The editor, presentation, PDF and
// PPTX all draw from layoutSlide, so a starter that lays out without overflow
// in every theme renders the same in all four.
const dir = new URL("../../server/internal/deck/starters/", import.meta.url);
const starters = readdirSync(dir).filter((f) => f.endsWith(".json")).map((f) => [f, JSON.parse(readFileSync(new URL(f, dir), "utf8"))]);
// A deliberately wide estimate (bold sans averages about 0.55 em per character).
const measure = (text, _slot, style, size) => text.length * (style.startsWith("bold") ? 0.6 : 0.55) * size;
const frames = (els) => Object.fromEntries(els.filter((e) => e.placeholder).map((e) => [e.placeholder, e.frame]));
const layouts = new Set(LAYOUTS.map((l) => l.id));

test("there are five starter decks", () => {
  assert.deepEqual(starters.map(([f]) => f).sort(), ["pitch.json", "project-update.json", "research-findings.json", "retro.json", "teaching.json"]);
});

for (const [file, deck] of starters) {
  test(`${file}: known layouts, placeholders where the layout puts them, no overflow in any theme`, () => {
    for (const s of deck.slides) {
      assert.ok(layouts.has(s.layout), `${s.id}: ${s.layout}`);
      if (s.layout === "blank") continue;
      const want = frames(placeholders(s.layout));
      for (const [role, f] of Object.entries(frames(s.elements))) {
        // Columns under a card may start lower than the layout's body; title and labels must match.
        if (role.startsWith("body")) assert.equal(f.x, want[role].x, `${s.id} ${role}`);
        else assert.deepEqual(f, want[role], `${s.id} ${role}`);
      }
    }
    for (const key of THEME_ORDER) {
      const themed = { ...deck, theme: THEMES[key] };
      for (const s of deck.slides) {
        const laid = layoutSlide(themed, s, measure);
        assert.deepEqual([...laid.overflow], [], `${file} ${s.id} overflows in ${key}`);
        assert.ok(laid.ops.length > 0, `${s.id} draws nothing`);
      }
    }
  });
}
