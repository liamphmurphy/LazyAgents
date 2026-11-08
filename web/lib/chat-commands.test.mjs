import assert from "node:assert/strict";
import test from "node:test";
import { commandMenu, literalBody, parseCommand } from "./chat-commands.ts";

test("parseCommand reads a one-line /command and its argument", () => {
  assert.deepEqual(parseCommand("/stop"), { name: "stop", arg: "" });
  assert.deepEqual(parseCommand("/Persona  Bea Arthur \n"), { name: "persona", arg: "Bea Arthur" });
});

test("parseCommand leaves ordinary text alone", () => {
  assert.equal(parseCommand("/usr/bin is missing"), null);
  assert.equal(parseCommand("see /stop"), null);
  assert.equal(parseCommand("/stop\nand more"), null);
  assert.equal(parseCommand("//stop"), null);
  assert.equal(parseCommand("/"), null);
});

test("literalBody drops the escaping slash only", () => {
  assert.equal(literalBody("//stop"), "/stop");
  assert.equal(literalBody("/stop"), "/stop");
  assert.equal(literalBody("hello // there"), "hello // there");
});

test("commandMenu offers names, then arguments of commands that take one", () => {
  assert.deepEqual(commandMenu("/"), { stage: "command", query: "" });
  assert.deepEqual(commandMenu("/mo"), { stage: "command", query: "mo" });
  assert.deepEqual(commandMenu("/model "), { stage: "arg", name: "model", query: "" });
  assert.deepEqual(commandMenu("/MODEL op"), { stage: "arg", name: "model", query: "op" });
  assert.equal(commandMenu("/stop "), null);
  assert.equal(commandMenu("/nope x"), null);
  assert.equal(commandMenu("//"), null);
  assert.equal(commandMenu("/usr/bin"), null);
  assert.equal(commandMenu("hello"), null);
});

test("/workflow takes a workflow to pick", () => {
  assert.deepEqual(commandMenu("/workflow "), { stage: "arg", name: "workflow", query: "" });
  assert.deepEqual(parseCommand("/workflow Research to wiki"), { name: "workflow", arg: "Research to wiki" });
});
