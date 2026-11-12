import test from "node:test";
import assert from "node:assert/strict";
import { hoverReveal, hoverShow, hoverVisible } from "./reveal.ts";

test("every hover reveal also shows on devices without hover", () => {
  for (const cls of [hoverReveal(), hoverReveal("msg"), hoverReveal("view"), hoverShow(), hoverVisible()]) {
    assert.match(cls, /\[@media\(hover:none\)\]:(opacity-100|visible)/);
  }
});

test("the group name picks the group-hover variant", () => {
  assert.equal(hoverReveal(), "opacity-0 group-hover:opacity-100 [@media(hover:none)]:opacity-100");
  assert.match(hoverReveal("msg"), /group-hover\/msg:opacity-100/);
  assert.match(hoverReveal("view"), /group-hover\/view:opacity-100/);
});
