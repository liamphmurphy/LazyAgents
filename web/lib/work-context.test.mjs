import test from "node:test";
import assert from "node:assert/strict";
import { WORK_CONTEXT_MARKER, parsePin, sameSubject, withWorkContext, workContextLine, workSubjectLabel } from "./work-context.ts";

const run = { kind: "run", id: "r1", title: "Welcome flow checks", agent: "Atlas", ticketKey: "WEB-42" };
const ticket = { kind: "ticket", key: "WEB-42", title: "Simplify the first welcome step" };

test("the context names what to look up", () => {
  assert.equal(workContextLine(run), "Viewing run r1 (“Welcome flow checks”) by Atlas for ticket WEB-42.");
  assert.equal(workContextLine({ ...run, ticketKey: undefined }), "Viewing run r1 (“Welcome flow checks”) by Atlas.");
  assert.equal(workContextLine(ticket), "Viewing ticket WEB-42 (“Simplify the first welcome step”).");
});

test("the context follows the marker the server cuts at, and is added once", () => {
  const body = withWorkContext("Is this safe?", run);
  assert.equal(body, `Is this safe?${WORK_CONTEXT_MARKER}Viewing run r1 (“Welcome flow checks”) by Atlas for ticket WEB-42.`);
  assert.equal(WORK_CONTEXT_MARKER, "\n\n<!-- vesper:work-context -->\n");
  assert.equal(withWorkContext(body, run), body);
});

test("the chip says which item the message is about", () => {
  assert.equal(workSubjectLabel(run), "WEB-42 · Welcome flow checks");
  assert.equal(workSubjectLabel({ ...run, ticketKey: undefined }), "Welcome flow checks");
  assert.equal(workSubjectLabel(ticket), "WEB-42 · Simplify the first welcome step");
});

test("a pinned item rides along with what's in view, named as pinned", () => {
  const body = withWorkContext("Compare them", run, ticket);
  assert.equal(body, `Compare them${WORK_CONTEXT_MARKER}Pinned ticket WEB-42 (“Simplify the first welcome step”).\nViewing run r1 (“Welcome flow checks”) by Atlas for ticket WEB-42.`);
  assert.equal(withWorkContext(body, run, ticket), body);
  // Pinned and in view at once: one line, marked pinned.
  assert.equal(withWorkContext("Hi", ticket, ticket), `Hi${WORK_CONTEXT_MARKER}Pinned ticket WEB-42 (“Simplify the first welcome step”).`);
});

test("a stored pin is read back, and anything malformed is no pin", () => {
  assert.deepEqual(parsePin(JSON.stringify(run)), run);
  assert.deepEqual(parsePin(JSON.stringify(ticket)), ticket);
  for (const raw of [null, "", "{", JSON.stringify({ kind: "run", id: 1 }), JSON.stringify({ kind: "page", id: "p" })]) assert.equal(parsePin(raw), undefined);
  assert.equal(sameSubject(run, { ...run, title: "renamed" }), true);
  assert.equal(sameSubject(run, ticket), false);
});

test("with nothing in view only the pin goes, and with no pin the body goes as it is", () => {
  const ticket = { kind: "ticket", key: "WEB-9", title: "Fix login" };
  assert.equal(withWorkContext("Hello", undefined), "Hello");
  const pinnedOnly = withWorkContext("What's left?", undefined, ticket);
  assert.equal(pinnedOnly, `What's left?${WORK_CONTEXT_MARKER}${workContextLine(ticket, true)}`);
  assert.equal(withWorkContext(pinnedOnly, undefined, ticket), pinnedOnly);
});
