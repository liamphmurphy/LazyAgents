import test from "node:test";
import assert from "node:assert/strict";
import { layoutSwitchHref, ticketsHref, ticketsLayout } from "./tickets-layout.ts";

test("the list is the default layout, and unknown layouts fall back to it", () => {
  assert.equal(ticketsLayout(null), "list");
  assert.equal(ticketsLayout("board"), "board");
  assert.equal(ticketsLayout("epics"), "epics");
  assert.equal(ticketsLayout("map"), "list");
});

test("ticketsHref leaves the list's layout out so old /tickets links stay the list", () => {
  assert.equal(ticketsHref("list"), "/tickets");
  assert.equal(ticketsHref("list", { project: "WEB", view: "active" }), "/tickets?project=WEB&view=active");
  assert.equal(ticketsHref("board", { project: "WEB", epic: undefined }), "/tickets?layout=board&project=WEB");
  assert.equal(ticketsHref("board", new URLSearchParams("layout=epics&project=WEB&tab=map")), "/tickets?layout=board&project=WEB&tab=map");
});

test("switching layouts keeps the project and shared filters, not layout-only ones", () => {
  const from = new URLSearchParams("layout=board&project=WEB&epic=e1&assignee=builder&tab=map&ticket=WEB-1");
  assert.equal(layoutSwitchHref("list", from), "/tickets?project=WEB&epic=e1&assignee=builder");
  assert.equal(layoutSwitchHref("epics", from), "/tickets?layout=epics&project=WEB");
  assert.equal(layoutSwitchHref("board", new URLSearchParams("view=active&q=x")), "/tickets?layout=board");
});
