import { expect, test } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installChatApi, persona } from "./mock-chat-api";

type Space = components["schemas"]["ChatSpace"];
const space = (id: string, name: string, over: Partial<Space> = {}): Space => ({
  id, name, description: "", color: "blue", instructions: "", connector_ids: [], space_memory: false, chat_count: 0, source_count: 0, memory_count: 0,
  inline_tokens: 0, created_at: "2026-11-01T09:00:00.000Z", updated_at: "2026-11-01T09:00:00.000Z", ...over,
});
// Named as the mock bootstrap names its WEB project: a space and its work share one name.
const WEBAPP = space("11111111-1111-4111-8111-000000000001", "Web app", {
  project: "WEB", work: { key: "WEB", repo_url: "https://github.com/acme/web.git", default_branch: "main", archived: false, revision: 1 },
});
const HEALTH = space("11111111-1111-4111-8111-000000000002", "Health", { color: "green", project: "WEB" });
const ATLAS = persona(1, { name: "Atlas", is_default: true });

const work = (page: import("@playwright/test").Page) => page.getByRole("region", { name: "Work" });

// LAZYAGENTS-213 frames A and B: a space's setup is the About tab of its companion panel, where
// Work tracks a key and a repository; tracked spaces get tabs to their tickets, epics and runs.
test("a space starts tracking work from its About panel, then links to its tickets, epics and runs", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS], spaces: [WEBAPP, HEALTH] });
  await page.goto(`/spaces/${HEALTH.id}`);
  await expect(page.getByRole("heading", { level: 1, name: "Health" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Health views" })).toHaveCount(0);
  await expect(work(page)).toContainText("Not tracked");
  await expect(work(page)).toContainText("new tickets from its chats go to Web app");

  await work(page).getByRole("button", { name: "Track work…" }).click();
  await expect(work(page).getByLabel("Key")).toHaveValue("HEAL");
  await work(page).getByLabel("Key").fill("fit");
  await work(page).getByLabel("Repository").fill("https://gitlab.com/me/fit");
  await expect(work(page)).toContainText("Only GitHub repositories work");
  await expect(work(page).getByRole("button", { name: "Track work" })).toBeDisabled();
  await work(page).getByLabel("Repository").fill("https://github.com/me/fit");
  await expect(work(page)).toContainText("Saved as https://github.com/me/fit.git");
  await work(page).getByRole("button", { name: "Track work" }).click();

  await expect(work(page)).toContainText("Tracked");
  await expect(work(page)).toContainText("FIT");
  expect(mock.workRequests).toContain(`PUT /chat-spaces/${HEALTH.id}/work {"key":"FIT","repo_url":"https://github.com/me/fit"}`);
  const views = page.getByRole("navigation", { name: "Health views" });
  await expect(views.getByRole("link", { name: "Tickets" })).toHaveAttribute("href", "/tickets?project=FIT");
  await expect(views.getByRole("link", { name: "Runs" })).toHaveAttribute("href", "/runs?project=FIT");
  // The left column marks it with its key.
  await expect(page.getByRole("region", { name: "Spaces" }).getByRole("link", { name: /Health\s*FIT/ })).toBeVisible();
  expect(mock.unhandled).toEqual([]);
});

test("a space's work changes its repository, and stopping is refused while it has tickets", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS], spaces: [WEBAPP, HEALTH], workWithTickets: ["WEB"] });
  await page.goto(`/spaces/${WEBAPP.id}`);
  await expect(work(page)).toContainText("acme/web");

  await work(page).getByRole("button", { name: "Edit" }).click();
  await work(page).getByLabel("Repository").fill("git@github.com:acme/site.git");
  await work(page).getByRole("button", { name: "Save" }).click();
  await expect(work(page)).toContainText("acme/site");
  expect(mock.workRequests.at(-1)).toContain('"repo_url":"git@github.com:acme/site.git"');

  await work(page).getByRole("button", { name: "Edit" }).click();
  await work(page).getByRole("button", { name: "Stop tracking work" }).click();
  await expect(work(page).getByRole("alert")).toContainText("WEB has tickets, so it keeps tracking work; archive it instead");

  // Deleting is offered only once it doesn't track work.
  await page.getByRole("button", { name: "More for this space" }).click();
  await page.getByRole("menuitem", { name: "Delete space…" }).click();
  await expect(page.getByRole("dialog")).toContainText("Web app tracks work (WEB), so it stays while it does.");
  await expect(page.getByRole("dialog").getByRole("button", { name: "Delete space" })).toBeDisabled();
  expect(mock.unhandled).toEqual([]);
});
