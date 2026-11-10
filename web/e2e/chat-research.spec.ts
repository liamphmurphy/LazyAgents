import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Deep research: approve a plan in the chat, follow its progress, read the report.

const now = () => new Date().toISOString();
const task = (label: string, state: string, extra = {}) => ({ label, question: "q", state, sources_read: 4, findings: 2, ...extra });

async function open(page: Page) {
  const mock = await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  const box = page.getByRole("combobox", { name: "Message" });
  await box.fill("research: local tts");
  await page.getByRole("button", { name: "Research", exact: true }).click();
  await expect(page.getByRole("button", { name: "Plan it" })).toBeVisible();
  await box.press("Enter");
  await expect(page.getByTestId("research-plan")).toBeVisible();
  return mock;
}

/** Serves a job once the plan starts; `job` is read fresh so a test can move it along. */
async function serve(page: Page, get: (message: string) => object, posts: unknown[]) {
  let message = "";
  await page.route(/\/api\/v1\/chats\/[^/]+\/research(\/.*)?$/, async (route: Route) => {
    const req = route.request();
    if (req.method() === "POST" && req.url().endsWith("/research")) {
      posts.push(req.postDataJSON());
      message = req.postDataJSON().message_id;
      return route.fulfill({ status: 201, json: get(message) });
    }
    if (req.method() === "POST") {
      posts.push(req.url().split("/").at(-1));
      return route.fulfill({ json: get(message) });
    }
    return route.fulfill({ json: { jobs: message ? [get(message)] : [] } });
  });
}

const base = (message: string) => ({ id: "11111111-1111-4111-8111-111111111111", message_id: message, title: "Local TTS", max_sources: 60, max_minutes: 30, report_length: "standard", report_tone: "report", sources_read: 8, spent_usd: 1.25, created_at: now() });

test("Research sends one message asking for a plan, then resets", async ({ page }) => {
  const mock = await open(page);
  expect(mock.posted.at(-1)?.body).toMatchObject({ body: "research: local tts", research: true });
  await expect(page.getByRole("button", { name: "Research", exact: true })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByTestId("research-question")).toHaveCount(2);
});

test("an edited plan with no spend limit starts research, then shows progress", async ({ page }) => {
  await open(page);
  const posts: { max_cost_usd?: number; questions?: unknown[]; max_sources?: number; report_length?: string; report_tone?: string }[] = [];
  await serve(page, (m) => ({ ...base(m), state: "running", tasks: [task("Quality", "done"), task("Latency", "running", { reading: "https://github.com/hexgrad/kokoro" }), task("Licensing", "queued", { sources_read: 0, findings: 0 })] }), posts as unknown[]);

  const plan = page.getByTestId("research-plan");
  await plan.getByRole("button", { name: "Change it" }).click();
  await plan.getByRole("button", { name: "+ Add a question" }).click();
  await plan.getByRole("textbox", { name: "Label 3" }).fill("Licensing");
  await plan.getByRole("textbox", { name: "Question 3" }).fill("Which licenses allow paid use?");
  await plan.getByRole("spinbutton", { name: "Sources, up to" }).fill("500");
  await expect(plan.getByRole("button", { name: "Start research" })).toBeDisabled();
  await plan.getByRole("spinbutton", { name: "Sources, up to" }).fill("80");
  await plan.getByRole("button", { name: "Unlimited" }).click();
  await expect(plan).toContainText("Time and source limits still apply");
  await plan.getByRole("group", { name: "Length" }).getByRole("button", { name: "Detailed" }).click();
  await plan.getByRole("group", { name: "Tone" }).getByRole("button", { name: "Essay" }).click();
  await plan.getByRole("button", { name: "Start research" }).click();

  const progress = page.getByTestId("research-progress");
  await expect(progress).toContainText("no spend limit");
  await expect(progress.getByTestId("research-task")).toHaveCount(3);
  await expect(progress.getByTestId("research-task").nth(1)).toContainText("Reading github.com");
  await expect(progress.getByTestId("research-task").nth(2)).toContainText("Queued behind other work");
  expect(posts[0].max_cost_usd).toBeUndefined();
  expect(posts[0].questions).toHaveLength(3);
  expect(posts[0].max_sources).toBe(80);
  expect(posts[0].report_length).toBe("detailed");
  expect(posts[0].report_tone).toBe("essay");

  await progress.getByRole("button", { name: "Finish with this" }).click();
  await expect.poll(() => posts.at(-1)).toBe("finish");
});

test("a finished report shows its card, with cost hidden when unlimited", async ({ page }) => {
  await open(page);
  const report = {
    bottom_line: "Kokoro is the best fit today.",
    key_findings: [{ text: "It runs fast", sources: [1] }],
    disagreements: [{ topic: "Latency", detail: "Sources range from 90 to 240 ms." }],
    sources: [{ n: 1, title: "Kokoro repo", url: "https://github.com/hexgrad/kokoro" }],
    body_md: "# Report\n\n## Comparison\nx\n\n## Not covered\ny\n",
  };
  const posts: unknown[] = [];
  await serve(page, (m) => ({ ...base(m), state: "done", tasks: [task("Quality", "done")], artifact_id: "22222222-2222-4222-8222-222222222222", finished_at: now(), report }), posts);
  await page.getByTestId("research-plan").getByRole("button", { name: "Start research" }).click();

  const card = page.getByTestId("research-report");
  await expect(card).toContainText("Kokoro is the best fit today.");
  await expect(card.getByTestId("report-stats")).not.toContainText("$");
  await expect(card.getByTestId("report-cite")).toHaveAttribute("href", "https://github.com/hexgrad/kokoro");
  await expect(card.getByTestId("report-disagreement")).toContainText("90 to 240");
  await expect(card.getByRole("link", { name: "Open report" })).toHaveAttribute("href", "/artifacts/22222222-2222-4222-8222-222222222222");
  await expect(card.getByRole("link", { name: "Not covered" })).toHaveAttribute("href", /#not-covered$/);
  await card.getByRole("button", { name: "1 sources" }).click();
  await expect(card.getByTestId("report-source")).toContainText("Kokoro repo");

  await card.getByRole("button", { name: "Research more" }).click();
  await expect(page.getByRole("button", { name: "Plan it" })).toBeVisible();
  await expect(page.getByTestId("context-chip")).toContainText("Local TTS");
});

test("a stopped job says so instead of offering the plan again", async ({ page }) => {
  await open(page);
  await serve(page, (m) => ({ ...base(m), state: "stopped", tasks: [] }), []);
  await page.getByTestId("research-plan").getByRole("button", { name: "Start research" }).click();
  await expect(page.getByTestId("research-ended")).toContainText("No report was written");
});
