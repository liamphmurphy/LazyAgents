import { expect, test, type Page } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

// Several personas in one chat: @mentions in the composer bring another persona
// in, and a persona's proposal for a new persona waits for the person's approval.

const ATLAS = persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true });
const NOVA = persona(2, { name: "Nova", tone: "playful", color: "#f472b6" });

const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });

async function openChat(page: Page) {
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("hello");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  await expect(page.getByTestId("assistant-message").first()).toBeVisible();
  return page.url().match(/\/chat\/([0-9a-f-]+)/)![1];
}

test("typing @ suggests personas and Enter inserts the mention", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  const id = await openChat(page);

  await composer(page).fill("@No");
  const list = page.getByTestId("mention-suggestions");
  await expect(list).toBeVisible();
  await expect(list.getByRole("option")).toHaveCount(1);
  await expect(list).toContainText("Nova");
  await composer(page).press("Enter");
  await expect(list).toHaveCount(0);
  await expect(composer(page)).toHaveValue("@Nova ");

  await composer(page).pressSequentially("what do you think?");
  await composer(page).press("Enter");
  await expect.poll(() => mock.posted.filter((p) => p.chat === id).map((p) => p.body.body)).toContain("@Nova what do you think?");
});

test("a proposed persona is created only when the person approves it", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  const id = await openChat(page);
  mock.proposals.push({
    id: "77777777-7777-4777-8777-777777777777", chat_id: id, name: "Sage", tone: "concise", color: "#6fcf97",
    custom_instructions: "Answer in three bullet points.", reason: "You asked for a terse reviewer.", state: "pending",
    proposed_by: { id: ATLAS.id, name: ATLAS.name, color: ATLAS.color }, created_at: "2026-11-07T10:00:00Z",
  });
  await page.reload();

  const card = page.getByTestId("persona-proposal");
  await expect(card).toContainText("Atlas");
  await expect(card).toContainText("wants to create a new persona");
  await expect(card).toContainText("Sage");
  await expect(card).toContainText("You asked for a terse reviewer.");
  await card.getByRole("button", { name: "Approve Sage" }).click();
  await expect.poll(() => mock.decisions).toEqual([{ id: "77777777-7777-4777-8777-777777777777", decision: "approve" }]);
  await expect(card).toHaveCount(0);
});
