import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Guided Learning: start a module, approve its outline, study a lesson.

const now = "2026-11-08T12:00:00Z";
const ID = "11111111-1111-4111-8111-111111111111";
const CHAT = "22222222-2222-4222-8222-222222222222";
const CHECK = "33333333-3333-4333-8333-333333333333";

const summary = (state: string, extra = {}) => ({ id: ID, topic: "Postgres MVCC", state, error: "", created_at: now, units: 2, lessons: 3, complete: 0, solid: 0, flagged: 0, minutes: 30, build_note: "", part: 0, parts: 1, studyable: state === "ready", ...extra });
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
  if (path.endsWith("/next-part")) {
    const c = course.current as ReturnType<typeof ready>;
    course.current = { ...c, summary: { ...c.summary, state: "scoping", part: 1 } };
    return json(route, course.current);
  }
  return json(route, course.current);
}

const DIAGRAM = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 90" width="320" height="90"><rect x="24" y="20" width="104" height="48" rx="8" fill="var(--viz-panel)" stroke="var(--viz-line)"/><g data-step="2"><rect x="192" y="20" width="104" height="48" rx="8" fill="var(--viz-1-soft)" stroke="var(--viz-1)"/></g></svg>`;
const CHART = JSON.stringify({ type: "line", title: "Dead tuples", categories: ["Mon", "Tue", "Wed"], series: [{ name: "Tuples", values: [10, 40, 25] }] });

test("a lesson teaches with step-throughs and charts, and the tutor can draw", async ({ page }) => {
  const course = { current: null as unknown };
  await mock(page, course, []);
  const data = ready();
  data.lessons[0] = lesson("snapshots", "Snapshots", {
    body: [
      { kind: "diagram", text: DIAGRAM, alt: "Two row versions", caption: "Each UPDATE adds a version", steps: ["The first version", "A second version"], provenance: "sourced", sources: [1] },
      { kind: "chart", text: CHART, alt: "Dead tuples rising midweek", provenance: "model_knowledge" },
    ],
    checks: [check()],
  });
  course.current = data;
  await page.route(new RegExp(`/api/v1/chats/${CHAT}/messages.*$`), (route) => json(route, { messages: [{ id: "m1", role: "assistant", body_md: "Picture it:\n\n```chart\n" + CHART + "\n```\n\n```choices\nPick\n- A\n```" }] }));
  await page.goto(`/learn/${ID}`);

  const article = page.getByRole("article");
  const diagram = article.getByTestId("svg-fence");
  await expect(diagram.getByRole("img", { name: /Two row versions. Step 1 of 2: The first version/ })).toBeVisible();
  await expect(diagram).toContainText("Each UPDATE adds a version");
  await expect(diagram.getByRole("link", { name: "[1]" })).toBeVisible();
  await expect(diagram.getByRole("tab", { name: "Code" })).toHaveCount(0);
  await diagram.getByRole("button", { name: "Next step" }).click();
  await expect(diagram.getByRole("img", { name: /Step 2 of 2: A second version/ })).toBeVisible();
  await expect(diagram.getByRole("button", { name: "Next step" })).toBeDisabled();

  const chart = article.getByTestId("chart");
  await expect(chart.getByRole("img", { name: "Dead tuples rising midweek" })).toBeVisible();
  await expect(article.getByText("MODEL KNOWLEDGE · NOT SOURCED")).toBeVisible();

  const tutor = page.getByRole("complementary", { name: "Tutor" });
  await expect(tutor.getByTestId("chart")).toBeVisible();
  await expect(tutor).toContainText("- A");
  await expect(tutor.getByRole("button", { name: "A", exact: true })).toHaveCount(0);
  await page.screenshot({ path: "test-results/learn-visuals.png", fullPage: true });
});

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
  const time = dialog.getByRole("slider", { name: "Time" });
  await expect(time).toHaveAttribute("aria-valuetext", "3 hours");
  await time.press("End");
  await expect(time).toHaveAttribute("aria-valuetext", "20 hours");
  await expect(dialog.getByText("In 5 parts")).toBeVisible();
  await page.screenshot({ path: "test-results/learn-guide-time.png", animations: "disabled" });
  await time.press("ArrowLeft");
  await time.press("ArrowLeft");
  await time.press("ArrowLeft");
  await expect(dialog.getByText("In 4 parts")).toBeVisible(); // 17 hours
  await time.press("Home");
  for (let i = 0; i < 5; i++) await time.press("ArrowRight");
  await expect(time).toHaveAttribute("aria-valuetext", "3 hours");
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

test("a module built in parts keeps studying while its next part is planned", async ({ page }) => {
  const course = { current: null as unknown };
  const posts: { url: string; body: unknown }[] = [];
  await mock(page, course, posts);
  const parts = [{ title: "Visibility and vacuum", summary: "How rows live", minutes: 600 }, { title: "Indexes", summary: "Finding rows fast", minutes: 600 }];
  const data = ready();
  course.current = { ...data, minutes_budget: 1200, outline: { ...outline, parts }, summary: { ...data.summary, parts: 2 } };
  await page.goto(`/learn/${ID}`);

  const next = page.getByTestId("next-part");
  await expect(next).toContainText("Part 2 of 2");
  await expect(next).toContainText("Indexes");
  await next.getByRole("button", { name: "Plan part 2 now" }).click();
  await expect(next).toContainText("Drafting its outline");
  expect(posts.map((p) => p.url)).toContain(`/learn/courses/${ID}/next-part`);
  // Studying carries on meanwhile.
  await expect(page.getByRole("heading", { name: "Snapshots" })).toBeVisible();
  await page.screenshot({ path: "test-results/learn-next-part.png", animations: "disabled" });

  const c = course.current as ReturnType<typeof ready>;
  course.current = { ...c, outline: { ...outline, parts, units: [...outline.units, { part: 1, title: "B-trees", question: "How do B-trees work?", lessons: [{ key: "btree", title: "B-trees", objectives: ["Walk a lookup"], prereqs: ["snapshots"], minutes: 20 }] }] }, summary: { ...c.summary, state: "outline_ready", part: 1, units: 1 } };
  await page.reload();
  await page.getByTestId("next-part").getByRole("link", { name: "Review its outline" }).click();
  await expect(page.getByText("Review part 2's outline.")).toBeVisible();
  await expect(page.getByText("Part 2 of 2: Indexes")).toBeVisible();
  await expect(page.getByText("Unit 3 · B-trees")).toBeVisible();
  await expect(page.getByText("Unit 1 · Visibility")).toHaveCount(0);
  await expect(page.getByText("needs 1.2")).toBeVisible();
  await page.screenshot({ path: "test-results/learn-part-outline.png", fullPage: true });
});
