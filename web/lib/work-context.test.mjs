import test from "node:test";
import assert from "node:assert/strict";
import { WORK_CONTEXT_MARKER, withWorkContext, workContextLine, workSubjectLabel } from "./work-context.ts";

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
