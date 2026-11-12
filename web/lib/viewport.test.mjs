import test from "node:test";
import assert from "node:assert/strict";
import { COMPACT, COMPACT_QUERY, mediaStore } from "./viewport.ts";

// A matchMedia stand-in that evaluates min-/max-width queries against a width.
function fakeWindow(width) {
  const lists = [];
  const win = {
    width,
    matchMedia(query) {
      const [, kind, px] = /\((min|max)-width:\s*([\d.]+)px\)/.exec(query);
      const listeners = new Set();
      const mq = {
        get matches() { return kind === "min" ? win.width >= Number(px) : win.width <= Number(px); },
        addEventListener: (_, fn) => listeners.add(fn),
        removeEventListener: (_, fn) => listeners.delete(fn),
        listeners,
      };
      lists.push(mq);
      return mq;
    },
    resize(w) {
      win.width = w;
      for (const mq of lists) for (const fn of mq.listeners) fn();
    },
    listenerCount: () => lists.reduce((n, mq) => n + mq.listeners.size, 0),
  };
  return win;
}

test("COMPACT is Tailwind's md breakpoint and the query is max-md", () => {
  assert.equal(COMPACT, 768);
  assert.equal(COMPACT_QUERY, "(max-width: 767.98px)");
});

test("the compact query matches below 768px only", () => {
  const cases = [[390, true], [767, true], [767.98, true], [768, false], [1024, false]];
  for (const [width, compact] of cases) {
    globalThis.window = fakeWindow(width);
    assert.equal(mediaStore(COMPACT_QUERY).getSnapshot(), compact, `${width}px`);
  }
});

test("the apps menu's 800px query is unchanged: wide from 800px", () => {
  for (const [width, wide] of [[768, false], [799, false], [800, true], [1280, true]]) {
    globalThis.window = fakeWindow(width);
    assert.equal(mediaStore("(min-width: 800px)").getSnapshot(), wide, `${width}px`);
  }
});

test("subscribe notifies on change and unsubscribes cleanly", () => {
  const win = (globalThis.window = fakeWindow(1024));
  const store = mediaStore(COMPACT_QUERY);
  let calls = 0;
  const unsubscribe = store.subscribe(() => calls++);
  win.resize(390);
  assert.equal(calls, 1);
  assert.equal(store.getSnapshot(), true);
  unsubscribe();
  assert.equal(win.listenerCount(), 0);
  win.resize(1024);
  assert.equal(calls, 1);
});
