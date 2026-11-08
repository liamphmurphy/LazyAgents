import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Guided Learning: start a module, approve its outline, study a lesson.

const now = "2026-11-08T12:00:00Z";
const ID = "11111111-1111-4111-8111-111111111111";
const CHAT = "22222222-2222-4222-8222-222222222222";
const CHECK = "33333333-3333-4333-8333-333333333333";

const summary = (state: string, extra = {}) => ({ id: ID, topic: "Postgres MVCC", state, error: "", created_at: now, units: 2, lessons: 3, complete: 0, solid: 0, flagged: 0, minutes: 30, build_note: "", ...extra });
const outline = {
  summary: "How row versions work.",
  units: [
    { title: "Visibility", question: "How do snapshots decide visibility?", lessons: [
      { key: "versions", title: "Row versions", objectives: ["Trace an UPDATE"], prereqs: [], minutes: 10 },
      { key: "snapshots", title: "Snapshots", objectives: ["Predict visibility"], prereqs: ["versions"], minutes: 10 },
    ] },
    { title: "Vacuum", question: "What does VACUUM reclaim?", lessons: [{ key: "vacuum", title: "Vacuum", objectives: ["Explain reclaim"], prereqs: ["snapshots"], minutes: 10 }] },
  ],
};
const base = { goal: "", level: "some", minutes_budget: 180, chat_id: CHAT, max_sources: 40, max_minutes: 30, sources: [] as unknown[], lessons: [] as unknown[] };
const lesson = (key: string, title: string, extra = {}) => ({ key, title, unit_title: "Visibility", objectives: ["Predict visibility"], prereqs: [], minutes: 8, status: "verified", issues: [], progress: "new", shaky_prereqs: [], body: [], checks: [], ...extra });
const check = (extra = {}) => ({ id: CHECK, objective: "Predict visibility", kind: "mcq", prompt: "Which version does T130 read?", options: ["v2", "v1", "Neither"], attempts: 0, solved: false, ...extra });
const ready = (chk = check()) => ({
  ...base,
  summary: summary("ready"),
  sources: [{ n: 1, title: "PostgreSQL docs", url: "https://www.postgresql.org/docs/" }],
  lessons: [
    lesson("snapshots", "Snapshots", { body: [{ kind: "text", text: "A snapshot records transactions in progress.", provenance: "sourced", sources: [1] }, { kind: "text", text: "Think of it as a list.", provenance: "model_knowledge" }], checks: [chk] }),
    lesson("vacuum", "Vacuum", { status: "flagged", issues: ["The claim about the OS is wrong"], shaky_prereqs: ["Snapshots"], unit_title: "Vacuum", checks: [check({ id: "44444444-4444-4444-8444-444444444444" })] }),
  ],
});

const json = (route: Route, body: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });

async function mock(page: Page, course: { current: unknown }, posts: { url: string; body: unknown }[]) {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/learn\/courses.*/, (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname.replace("/api/v1", "");
    if (req.method() !== "GET") posts.push({ url: path, body: req.postDataJSON() });
    return handle(route, path, req.method(), course);
  });
  await page.route(new RegExp(`/api/v1/chats/${CHAT}(/messages.*)?$`), (route) => (route.request().url().includes("/messages") ? json(route, { messages: [] }) : json(route, { id: CHAT, state: "idle" })));
}

function handle(route: Route, path: string, method: string, course: { current: unknown }) {
  if (path === "/learn/courses" && method === "GET") return json(route, []);
  if (path === "/learn/courses" && method === "POST") {
    course.current = { ...base, summary: summary("outline_ready"), outline };
    return json(route, course.current, 201);
  }
  if (path.endsWith("/approve")) {
    course.current = { ...base, summary: summary("researching", { build_note: "0 of 2 researchers done · 0 sources" }), outline };
    return json(route, course.current);
  }
  if (path.endsWith("/outline")) return json(route, { code: "invalid", message: "\"Row versions\" can't come before \"Snapshots\": it depends on it" }, 422);
  if (path.endsWith("/attempts")) {
    course.current = ready(check({ attempts: 1, solved: true, last_correct: true, reveal: { answer: ["v1"], rubric: "", explanation: "120 was in progress." } }));
    return json(route, { id: CHECK, correct: true }, 201);
  }
  if (path.endsWith("/tutor")) return route.fulfill({ status: 202 });
  return json(route, course.current);
}

test("a topic becomes an outline to approve", async ({ page }) => {
  const course = { current: null as unknown };
  const posts: { url: string; body: unknown }[] = [];
  await mock(page, course, posts);
  await page.goto("/learn");
  await expect(page.getByRole("link", { name: "Learn", exact: true })).toHaveAttribute("aria-current", "page");
  await page.screenshot({ path: "test-results/learn-home.png" });

  await page.getByRole("button", { name: "New Learning Module" }).click();
  const dialog = page.getByRole("dialog", { name: "New Learning Module" });
  const topic = dialog.getByLabel("What do you want to learn?");
  await expect(topic).toBeFocused();
  await expect(dialog.getByRole("button", { name: "Continue" })).toBeDisabled();
  await topic.fill("Postgres MVCC");
  await topic.press("Enter");

  const goal = dialog.getByLabel("What's it for?");
  await expect(goal).toBeFocused();
  await dialog.getByRole("button", { name: "Skip" }).click();

  const experienced = dialog.getByRole("radio", { name: /^Experienced/ });
  await expect(dialog.getByRole("radio", { name: /^Some background/ })).toBeChecked();
  await experienced.check({ force: true });
  await dialog.getByRole("button", { name: "Continue" }).click();
  await expect(dialog.getByRole("radio", { name: /^3 hours/ })).toBeChecked();
  await dialog.getByRole("button", { name: "Continue" }).click();
  await expect(dialog.getByText("Here's your module")).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Edit topic" })).toContainText("Postgres MVCC");
  await page.screenshot({ path: "test-results/learn-guide-review.png" });

  // Editing an answer goes back to that step and keeps the rest.
  await dialog.getByRole("button", { name: "Edit level" }).click();
  await expect(experienced).toBeChecked();
  await dialog.getByRole("button", { name: "Continue" }).click();
  await dialog.getByRole("button", { name: "Continue" }).click();

  await dialog.getByRole("button", { name: "Draft outline" }).click();
  await expect(page).toHaveURL(new RegExp(`/learn/${ID}`));
  await expect(page.getByText("Research starts when you approve it.")).toBeVisible();
  expect(posts[0].body).toMatchObject({ topic: "Postgres MVCC", level: "experienced", minutes_budget: 180 });
  expect(posts).toHaveLength(1);
  await page.screenshot({ path: "test-results/learn-outline.png", fullPage: true });

  await page.getByRole("button", { name: "Move Snapshots up" }).click();
  await expect(page.getByText("can't come before")).toBeVisible();
  expect(posts.at(-1)?.body).toMatchObject({ units: [{ lessons: ["snapshots", "versions"] }, { lessons: ["vacuum"] }] });

  await page.getByRole("button", { name: "Approve and start research" }).click();
  await expect(page.getByText("0 of 2 researchers done")).toBeVisible();
  expect(posts.at(-1)?.body).toMatchObject({ max_sources: 40, max_minutes: 30 });
});

test("a lesson shows provenance, grades a check and reaches the tutor", async ({ page }) => {
  const course = { current: ready() as unknown };
  const posts: { url: string; body: unknown }[] = [];
  await mock(page, course, posts);
  await page.goto(`/learn/${ID}`);
  await expect(page.getByRole("heading", { name: "Snapshots" })).toBeVisible();
  await expect(page.getByText("MODEL KNOWLEDGE · NOT SOURCED")).toBeVisible();
  await page.getByRole("radio", { name: "v1" }).check();
  await page.screenshot({ path: "test-results/learn-lesson.png" });
  await page.getByRole("button", { name: "Check answer" }).click();
  await expect(page.getByText("120 was in progress.")).toBeVisible();
  expect(posts.at(-1)?.body).toMatchObject({ response: ["v1"], confidence: 2 });

  await page.getByRole("textbox", { name: "Ask about this lesson" }).fill("why?");
  await page.getByRole("textbox", { name: "Ask about this lesson" }).press("Enter");
  await expect.poll(() => posts.at(-1)?.url).toContain("/tutor");
  expect(posts.at(-1)?.body).toMatchObject({ body: "why?", lesson: "snapshots" });

  await page.getByRole("button", { name: /^Vacuum/ }).click();
  await expect(page.getByText("The claim about the OS is wrong")).toBeVisible();
  await expect(page.getByText("Prerequisite shaky")).toBeVisible();
});
