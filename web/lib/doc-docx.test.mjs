import test from "node:test";
import assert from "node:assert/strict";
import * as docx from "docx";
import { unzipSync, strFromU8 } from "fflate";
import { parseDoc } from "./doc-markdown.ts";
import { toDocx } from "./doc-docx.ts";

const md = `# Lisbon trip plan

Four days in **late March**, see [the map](https://example.com/map).

1. Book flights
2. Book a stay

- Pack light
  - One bag

- [x] Passport

> Slow down.

\`\`\`
const x = 1;
\`\`\`

| Day | Plan |
|---|---|
| Fri | Alfama |
`;

async function documentXml(markdown) {
  const buf = await docx.Packer.toBuffer(toDocx(docx, parseDoc(markdown), "Lisbon"));
  return strFromU8(unzipSync(new Uint8Array(buf))["word/document.xml"]);
}

test("a Word export keeps the document's structure and formatting", async () => {
  const xml = await documentXml(md);
  assert.match(xml, /<w:pStyle w:val="Heading1"\/>.*?Lisbon trip plan/);
  assert.match(xml, /<w:b\/>.*?late March/);
  assert.match(xml, /<w:hyperlink[^>]*>.*?the map/);
  assert.match(xml, /<w:numPr><w:ilvl w:val="0"\/><w:numId w:val="\d+"\/><\/w:numPr>.*?Book flights/);
  assert.match(xml, /<w:ilvl w:val="1"\/>.*?One bag/, "nested bullets indent a level");
  assert.match(xml, /☑ .*?Passport/);
  assert.match(xml, /Menlo.*?const x = 1;/);
  assert.match(xml, /<w:tbl>.*Fri.*Alfama.*<\/w:tbl>/s);
  assert.match(xml, /Slow down\./);
});

test("an empty document still makes a valid file", async () => {
  assert.match(await documentXml(""), /<w:body>/);
});
