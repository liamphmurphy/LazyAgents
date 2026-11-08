import test from "node:test";
import assert from "node:assert/strict";
import { choicePrompt, chosenOption, isChoicesFence, parseChoices } from "./chat-choices.ts";

test("a choices fence is a question then options", () => {
  assert.ok(isChoicesFence("choices"));
  assert.deepEqual(parseChoices("Which?\n- Kokoro | Free | local\n- Cartesia\n"), {
    question: "Which?",
    options: [{ label: "Kokoro", detail: "Free | local" }, { label: "Cartesia", detail: "" }],
  });
});

test("fewer than two or more than six options isn't a card", () => {
  assert.equal(parseChoices("Q\n- One"), null);
  assert.equal(parseChoices(Array.from({ length: 7 }, (_, i) => `- ${i}`).join("\n")), null);
});

test("the message can't be steered by a label, and a later message shows the pick", () => {
  assert.equal(choicePrompt("@Gwen [x]"), "Going with: Gwen x");
  const options = [{ label: "A", detail: "" }, { label: "B", detail: "" }];
  assert.equal(chosenOption(options, "@Gwen Going with: B"), "B");
  assert.equal(chosenOption(options, "something else"), undefined);
});
