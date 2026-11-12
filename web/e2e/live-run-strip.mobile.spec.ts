import { expect, test, type Page } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installChatApi, openEmptyChat } from "./mock-chat-api";

// Mobile (390×844): the live-run strip above the chat composer stands in for the activity orb.
type Run = components["schemas"]["Run"];

const step = (position: number, state: components["schemas"]["PlanStep"]["state"]) => ({ position, text: `Step ${position}`, state });

function run(id: string, ticket: string, agent: string, startedAt: string, plan = [step(1, "done"), step(2, "current"), step(3, "pending")]): Run {
  return {
    id, ticket_key: ticket, purpose: "ticket", agent: { id: `a-${id}`, name: agent, handle: agent.toLowerCase(), color_index: 1 }, mode: "managed",
    state: "running", harness: "claude-code", model: "m", started_at: startedAt, plan, scopes: [], approval_scopes: [],
  } as unknown as Run;
}

/** Serves `runs` as the live runs; registered after installChatApi so it wins. */
async function serveLiveRuns(page: Page, runs: Run[]) {
  await page.route(/\/api\/v1\/runs\?/, (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ runs, has_more: false }) }));
}

const runA = "00000000-0000-4000-8000-0000000000a1";
const runB = "00000000-0000-4000-8000-0000000000b2";

test("no live run: no strip and no orb", async ({ page }) => {
  await installChatApi(page);
  await serveLiveRuns(page, []);
  await openEmptyChat(page);
  await expect(page.getByRole("combobox", { name: "Message" })).toBeVisible();
  await expect(page.getByTestId("live-run-strip")).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Activity" })).toHaveCount(0);
});

test("one live run: the strip names agent, ticket and step and opens the run", async ({ page }) => {
  await installChatApi(page);
  await serveLiveRuns(page, [run(runA, "VESPER-11", "Keel", new Date().toISOString())]);
  await openEmptyChat(page);
  const strip = page.getByTestId("live-run-strip");
  await expect(strip).toContainText("Keel");
  await expect(strip).toContainText("VESPER-11");
  await expect(strip).toContainText("step 2 of 3");
  await expect(strip).not.toContainText("more");
  await expect(strip).toBeInViewport();
  await expect(page.getByRole("region", { name: "Activity" })).toHaveCount(0);
  await page.getByRole("link", { name: "Keel on VESPER-11, step 2 of 3" }).click();
  await expect(page).toHaveURL(new RegExp(`/runs/${runA}$`));
});

test("several live runs: the newest shows with +N more, which opens Runs", async ({ page }) => {
  await installChatApi(page);
  const earlier = new Date(Date.now() - 60_000).toISOString();
  await serveLiveRuns(page, [run(runA, "VESPER-11", "Keel", earlier), run(runB, "VESPER-12", "Nova", new Date().toISOString(), [step(1, "current"), step(2, "pending")])]);
  await openEmptyChat(page);
  const strip = page.getByTestId("live-run-strip");
  await expect(strip).toContainText("Nova");
  await expect(strip).toContainText("VESPER-12");
  await expect(strip).toContainText("step 1 of 2");
  await strip.getByRole("link", { name: "+1 more" }).click();
  await expect(page).toHaveURL(/\/runs$/);
});
