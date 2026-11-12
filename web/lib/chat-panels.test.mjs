import assert from "node:assert/strict";
import test from "node:test";
import { activePanel, closePanel, dragCloses, hideSheet, openPanels, openThreads, sheetHidden, switchPanel } from "./chat-panels.ts";

const D = "01a1bbdd-4937-7baa-af62-b3726025d331";
const C = "01a1bbdd-4937-7baa-af62-b3726025d332";
const q = (s) => new URLSearchParams(s);

test("nothing is open by default", () => {
  assert.deepEqual(openPanels(q(""), [D]), []);
  assert.equal(activePanel(q(""), []), undefined);
});

test("opening: a design focus, a fullscreen canvas and the playground each open their panel", () => {
  assert.deepEqual(openPanels(q(`canvas=${D}`), [D]), ["design"]);
  // A focus on a canvas that isn't a design expands its card in the chat, not a panel.
  assert.deepEqual(openPanels(q(`canvas=${C}`), [D]), []);
  assert.deepEqual(openPanels(q("canvas=none"), [D]), []);
  assert.deepEqual(openPanels(q(`canvas=${C}&canvas_view=full`), [D]), ["canvas"]);
  // A fullscreen design shows as the canvas, not beside the chat.
  assert.deepEqual(openPanels(q(`canvas=${D}&canvas_view=full`), [D]), ["canvas"]);
  assert.deepEqual(openPanels(q("playground=open"), []), ["playground"]);
  assert.deepEqual(openPanels(q("playground=full"), []), ["playground"]);
  assert.deepEqual(openPanels(q(`canvas=${D}&playground=open`), [D]), ["design", "playground"]);
});

test("switchPanel opens the panel it switches to", () => {
  const pg = switchPanel(q(""), "playground");
  assert.deepEqual(openPanels(pg, []), ["playground"]);
  assert.equal(activePanel(pg, openPanels(pg, [])), "playground");
  // Already full: stays full.
  assert.equal(switchPanel(q("playground=full"), "playground").get("playground"), "full");

  const cv = switchPanel(q(`canvas=${C}`), "canvas");
  assert.equal(cv.get("canvas_view"), "full");
  assert.equal(cv.get("canvas"), C);

  const ds = switchPanel(q("canvas_view=full"), "design", { designId: D });
  assert.deepEqual(openPanels(ds, [D]), ["design"]);
  // No design to open: nothing changes.
  assert.equal(switchPanel(q("a=1"), "design").toString(), "a=1");
});

test("switching between open panels changes which one the sheet shows", () => {
  let s = q(`canvas=${D}&playground=open`);
  assert.equal(activePanel(s, openPanels(s, [D])), "playground");
  s = switchPanel(s, "design", { designId: D });
  assert.deepEqual(openPanels(s, [D]), ["design", "playground"]);
  assert.equal(activePanel(s, openPanels(s, [D])), "design");
  s = switchPanel(s, "playground");
  assert.equal(activePanel(s, openPanels(s, [D])), "playground");
  // Switching to the canvas puts the design fullscreen.
  s = switchPanel(s, "canvas");
  assert.deepEqual(openPanels(s, [D]), ["canvas", "playground"]);
  assert.equal(activePanel(s, openPanels(s, [D])), "canvas");
});

test("activePanel falls back to the frontmost open panel when the one picked is closed", () => {
  assert.equal(activePanel(q("panel=design"), ["playground"]), "playground");
  assert.equal(activePanel(q(""), ["design", "canvas", "playground"]), "canvas");
  assert.equal(activePanel(q(""), ["design"]), "design");
});

test("closePanel closes only that panel", () => {
  let s = q(`canvas=${D}&playground=open&playground_tab=logs&panel=playground`);
  s = closePanel(s, "playground");
  assert.equal(s.get("playground"), null);
  assert.equal(s.get("playground_tab"), null);
  assert.equal(s.get("panel"), null);
  assert.deepEqual(openPanels(s, [D]), ["design"]);
  s = closePanel(s, "design");
  assert.equal(s.get("canvas"), "none");
  assert.deepEqual(openPanels(s, [D]), []);

  const full = closePanel(q(`canvas=${C}&canvas_view=full&panel=design`), "canvas");
  assert.equal(full.get("canvas_view"), null);
  assert.equal(full.get("canvas"), C);
  // A pick of another panel survives.
  assert.equal(full.get("panel"), "design");
});

test("a drag down closes the sheet past a quarter of its height, or as a quick flick", () => {
  assert.equal(dragCloses(100, 1000, 800), false);
  assert.equal(dragCloses(200, 1000, 800), true);
  // At least 120px on a short sheet.
  assert.equal(dragCloses(100, 1000, 300), false);
  assert.equal(dragCloses(130, 1000, 300), true);
  assert.equal(dragCloses(60, 80, 800), true);
  assert.equal(dragCloses(30, 10, 800), false);
  assert.equal(dragCloses(-200, 10, 800), false);
});

test("the Comments count leaves out resolved threads", () => {
  assert.equal(openThreads(undefined), 0);
  assert.equal(openThreads([{ status: "open" }, { status: "draft" }, { status: "addressed" }, { status: "resolved" }, { status: "open", parent_id: "x" }]), 3);
});

test("a dismissed sheet stays dismissed until a switch shows it again", () => {
  const hidden = hideSheet(q(`playground=open&panel=playground`));
  assert.equal(sheetHidden(hidden), true);
  // The panels stay open behind it.
  assert.deepEqual(openPanels(hidden, []), ["playground"]);
  const shown = switchPanel(hidden, "playground");
  assert.equal(sheetHidden(shown), false);
  assert.equal(shown.get("playground"), "open");
});
