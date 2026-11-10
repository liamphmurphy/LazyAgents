import test from "node:test";
import assert from "node:assert/strict";
import { editorKeeps, locateQuote, parseDoc, serializeDoc } from "./doc-markdown.ts";

const plan = `# Lisbon trip plan

Four days in **late March**, flying in _Friday_ morning. See [the map](https://example.com/map).

## Days

- Fri: Alfama
- Sat: Belém
  - pastéis

1. one
2. two

- [ ] book hotel
- [x] flights

> Slow down.

\`\`\`mermaid
graph TD; A-->B
\`\`\`

| Day | Plan |
|---|---|
| Fri | Alfama |

---

Prices like $20 stay as written.
`;

test("the editor keeps everything a document is usually made of", () => {
  assert.equal(editorKeeps(plan), true);
  // Spelling may change, meaning may not.
  const back = serializeDoc(parseDoc(plan));
  assert.match(back, /\*Friday\*/);
  assert.equal(editorKeeps(back), true);
});

test("the editor refuses Markdown it would lose", () => {
  assert.equal(editorKeeps("Some <span>raw html</span> here.\n"), false);
  assert.equal(editorKeeps("A claim.[^1]\n\n[^1]: The source.\n"), false);
});

test("a selection is found in its Markdown across the syntax the editor hides", () => {
  const at = (q, near) => {
    const r = locateQuote(plan, q, near);
    return r && plan.slice(r.start, r.end);
  };
  assert.equal(at("late March, flying in Friday"), "late March**, flying in _Friday");
  assert.equal(at("See the map."), "See [the map](https://example.com/map).");
  assert.equal(at("Alfama Sat: Belém"), "Alfama\n- Sat: Belém");
  assert.equal(at("one two"), "one\n2. two");
  assert.equal(at("not in it"), undefined);
});

test("with several matches, the one nearest the selection wins", () => {
  const md = "Alfama first.\n\nThen Alfama again.\n";
  assert.equal(locateQuote(md, "Alfama", 0).start, 0);
  assert.equal(locateQuote(md, "Alfama", 18).start, md.lastIndexOf("Alfama"));
});
