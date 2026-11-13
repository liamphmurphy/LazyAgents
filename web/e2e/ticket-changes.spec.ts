import { expect, test } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

const ATLAS = persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true });

const proposal = (id: string, extra: Record<string, unknown> = {}) => ({
  id,
  ticket_key: "PAY-12",
  ticket_title: "Refund webhook retries",
  tool: "update_ticket",
  state: "pending",
  created_at: new Date().toISOString(),
  persona: { id: ATLAS.id, name: ATLAS.name, color: ATLAS.color },
  changes: [
    { field: "status", label: "Status", before: "in_progress", after: "in_review" },
    { field: "description", label: "Description", before: "Retries fail.", after: "Retries fail after 3 attempts.\n\nRepro: replay event 42." },
  ],
  ...extra,
});

async function openChat(page: import("@playwright/test").Page) {
  await page.goto("/chat");
  await page.getByRole("combobox", { name: "Message" }).fill("move PAY-12 to review");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
}

test("a persona's proposed ticket edit is applied from its card", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS] });
  await openChat(page);
  mock.ticketChanges.push(proposal("22222222-2222-4222-8222-222222222221"));

  const card = page.getByTestId("ticket-change");
  await expect(card).toContainText("Atlas");
  await expect(card).toContainText("wants to change");
  await expect(card.getByRole("link", { name: "PAY-12" })).toHaveAttribute("href", "/tickets/PAY-12");
  // Short values read as before → after; long ones stack, with the current text folded away.
  await expect(card.getByTestId("ticket-change-row").first()).toContainText("in progress→in review");
  await expect(card).toContainText("Repro: replay event 42.");
  await expect(card.getByText("Current")).toBeVisible();

  await card.getByRole("button", { name: "Apply" }).click();
  await expect(card).toHaveCount(0);
  expect(mock.ticketChangeDecisions).toEqual(["allow"]);
  expect(mock.unhandled).toEqual([]);
});

test("a ticket edit the ticket no longer allows says why it wasn't applied", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS] });
  await openChat(page);
  mock.ticketChanges.push(proposal("22222222-2222-4222-8222-222222222222", { refuse: "revision: ticket revision is stale" }));
  mock.ticketChanges.push(proposal("22222222-2222-4222-8222-222222222223", { tool: "comment_ticket", changes: [{ field: "comment", label: "Comment", after: "Found it." }] }));

  const cards = page.getByTestId("ticket-change");
  await expect(cards).toHaveCount(2);
  await cards.first().getByRole("button", { name: "Apply" }).click();
  await expect(page.getByTestId("ticket-change-requests").getByRole("alert")).toContainText("Not applied: revision: ticket revision is stale");
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toContainText("wants to comment on");
  await cards.first().getByRole("button", { name: "Decline" }).click();
  await expect(cards).toHaveCount(0);
  expect(mock.ticketChangeDecisions).toEqual(["allow", "deny"]);
  expect(mock.unhandled).toEqual([]);
});
