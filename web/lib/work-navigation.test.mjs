import test from "node:test";
import assert from "node:assert/strict";
import { workHref, workTarget, workView } from "./work-navigation.ts";

test("opening a brief keeps the selected run and filters in a restorable URL", () => {
  const p = new URLSearchParams(
    "view=activity&run=r1&project=WEB&agent=builder&q=nav",
  );
  const href = workTarget("/tickets/WEB-1", p);
  assert.equal(
    href,
    "/work?view=activity&run=r1&project=WEB&agent=builder&q=nav&ticket=WEB-1&context=ticket",
  );
  assert.equal(
    workTarget("/epics/e1", new URLSearchParams(href.split("?")[1])),
    `${href.replace("context=ticket", "context=epic")}&epic=e1`,
  );
});

test("focus changes preserve context selections and closing context removes only the pane", () => {
  const p = new URLSearchParams(
    "run=r1&ticket=WEB-1&context=ticket&status=todo",
  );
  assert.equal(
    workHref(p, { view: "tickets", context: null }),
    "/work?run=r1&ticket=WEB-1&status=todo&view=tickets",
  );
  assert.equal(
    workTarget("/runs/r2", p),
    "/work?run=r2&ticket=WEB-1&status=todo&view=activity",
  );
  assert.equal(workTarget("/tickets/new", p), null);
  assert.equal(workTarget("/knowledge/p1", p), null);
  assert.equal(workView(new URLSearchParams("view=unknown")), "activity");
});
