import { expect, test, type Page } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

const panel = (page: Page) => page.getByRole("region", { name: "Personas", exact: true });
const editor = (page: Page) => page.getByRole("dialog");
// Next keeps the page we navigated from mounted but hidden, so count only what is on screen.
const rows = (page: Page) => page.locator('[data-testid="persona-row"]:visible');

async function openPanel(page: Page) {
  await page.goto("/chat?project=WEB");
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Personas", exact: true }).click();
  await expect(page).toHaveURL(/\/personas$/);
  await expect(panel(page)).toBeVisible();
}

/** What the shell's /events stream would send after a change somewhere else. */
async function pushPersonaUpdated(page: Page, ids: string[]) {
  await page.evaluate((ids) => {
    const data = JSON.stringify({ type: "persona.updated", ids });
    for (const s of (window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> }).__sse) s.onmessage?.({ data });
  }, ids);
}

const team = () => [
  persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true, chat_count: 12, last_used_at: "2026-11-07T10:00:00Z", custom_instructions: "Keep answers short and lead with the risk." }),
  persona(2, { name: "Nova", tone: "playful", color: "#f472b6", chat_count: 7, last_used_at: "2026-11-06T10:00:00Z" }),
  persona(3, { name: "Sage", tone: "concise", color: "#6fcf97", chat_count: 3, last_used_at: "2026-11-05T10:00:00Z" }),
];

test("with no personas the built-in Assistant is the default; New persona creates one", async ({ page }) => {
  const mock = await installChatApi(page);
  await openPanel(page);
  await expect(panel(page)).toContainText("No personas yet");
  await expect(panel(page)).toContainText("Built-in · Neutral tone · can’t be deleted");

  await panel(page).getByRole("button", { name: "New persona" }).first().click();
  await expect(editor(page).getByRole("heading", { name: "New persona" })).toBeVisible();
  await editor(page).getByLabel("Name").fill("Nova");
  await expect(editor(page)).toContainText("4 / 60");
  await editor(page).getByRole("radio", { name: "Playful" }).click();
  await editor(page).getByRole("radio", { name: "#f472b6" }).click();
  await editor(page).getByLabel("Custom instructions").fill("One emoji max.");
  await expect(editor(page)).toContainText("14 / 2000");
  const preview = editor(page).getByLabel("Message preview");
  await expect(preview).toContainText("Nova");
  await expect(preview).toContainText("Playful · now");
  await expect(preview.getByText("Nova", { exact: true })).toHaveCSS("color", "rgb(244, 114, 182)");
  await editor(page).getByLabel("Make this my default persona").check();
  await editor(page).getByRole("button", { name: "Save persona" }).click();

  await expect(editor(page)).toHaveCount(0);
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText("Nova");
  await expect(rows(page).first()).toContainText("Default");
  expect(mock.personas[0]).toMatchObject({ name: "Nova", tone: "playful", color: "#f472b6", custom_instructions: "One emoji max.", is_default: true });
  expect(mock.unhandled).toEqual([]);
});

test("color: the wheel's hex input validates #rrggbb, Apply sets it, and a low-contrast color warns", async ({ page }) => {
  const mock = await installChatApi(page, { personas: team() });
  await openPanel(page);
  await panel(page).getByRole("button", { name: "Edit Nova" }).click();

  await editor(page).getByRole("radio", { name: /Custom/ }).click();
  const hex = editor(page).getByLabel("Hex");
  await hex.fill("#5b3f");
  await expect(editor(page).getByRole("alert")).toContainText("#rrggbb");
  await expect(editor(page).getByRole("button", { name: "Apply" })).toBeDisabled();
  await expect(editor(page).getByRole("button", { name: "Save persona" })).toBeDisabled();

  await hex.fill("#5B3FA8");
  await editor(page).getByRole("button", { name: "Apply" }).click();
  await expect(editor(page).getByRole("radio", { name: "Custom · #5b3fa8" })).toHaveAttribute("aria-checked", "true");
  const warning = editor(page).getByRole("status").filter({ hasText: "Low contrast" });
  await expect(warning).toContainText("aim for 4.5:1");

  // The wheel sets the color too: the rim's right edge is red (hue 0°, full saturation).
  await editor(page).getByRole("radio", { name: /Custom/ }).click();
  const wheel = editor(page).getByRole("slider", { name: /Color wheel/ });
  const box = (await wheel.boundingBox())!;
  await page.mouse.click(box.x + box.width - 2, box.y + box.height / 2);
  await expect(hex).not.toHaveValue("#5b3fa8");
  const picked = await hex.inputValue();
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(picked.slice(i, i + 2), 16));
  expect(r).toBeGreaterThan(g + 80);
  expect(r).toBeGreaterThan(b + 80);
  await editor(page).getByRole("button", { name: "Apply" }).click();
  await expect(editor(page).getByRole("radio", { name: `Custom · ${picked}` })).toHaveAttribute("aria-checked", "true");

  // Back to the low-contrast color, then take the suggestion.
  await editor(page).getByRole("radio", { name: /Custom/ }).click();
  await hex.fill("#5b3fa8");
  await editor(page).getByRole("button", { name: "Apply" }).click();
  await warning.getByRole("button", { name: /^Use #/ }).click();
  await expect(warning).toHaveCount(0);
  await editor(page).getByRole("button", { name: "Save persona" }).click();
  await expect(editor(page)).toHaveCount(0);
  const nova = mock.personas.find((p) => p.name === "Nova")!;
  expect(nova.color).toMatch(/^#[0-9a-f]{6}$/);
  expect(nova.color).not.toBe("#5b3fa8");
  expect(mock.unhandled).toEqual([]);
});

test("avatar: client checks and the server's 415 show inline; initials stay on the persona color", async ({ page }) => {
  const mock = await installChatApi(page, { personas: team() });
  await openPanel(page);
  await panel(page).getByRole("button", { name: "Edit Sage" }).click();
  const file = editor(page).getByLabel("Avatar image");

  await file.setInputFiles({ name: "selfie.heic", mimeType: "image/heic", buffer: Buffer.from("x") });
  await expect(editor(page).getByRole("alert")).toContainText("selfie.heic: type not supported.");
  await file.setInputFiles({ name: "banner.png", mimeType: "image/png", buffer: Buffer.alloc(2 * 1_048_576, 1) });
  await expect(editor(page).getByRole("alert")).toContainText("banner.png is 2.0 MB.");
  await expect(editor(page).getByRole("alert")).toContainText("The limit is 1 MB");

  // Passes the client check, but the bytes aren't an image: the server says 415.
  await file.setInputFiles({ name: "fake.png", mimeType: "image/png", buffer: Buffer.from("not really a png") });
  await editor(page).getByRole("button", { name: "Save persona" }).click();
  await expect(editor(page).getByRole("alert")).toContainText("fake.png: type not supported.");
  await expect(editor(page)).toBeVisible();
  expect(mock.personas.find((p) => p.name === "Sage")!.avatar_url).toBeUndefined();

  // A real PNG uploads, shows in the preview, and the list shows it.
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", "base64");
  await file.setInputFiles({ name: "me.png", mimeType: "image/png", buffer: png });
  await expect(editor(page).getByLabel("Message preview").locator("img").first()).toHaveAttribute("src", /^blob:/);
  await editor(page).getByRole("button", { name: "Save persona" }).click();
  await expect(editor(page)).toHaveCount(0);
  expect(mock.personas.find((p) => p.name === "Sage")!.avatar_url).toMatch(/\/avatar\?v=/);
  await expect(rows(page).filter({ hasText: "Sage" }).locator("img")).toHaveCount(1);
  // The others have no image: their initial on the persona color.
  await expect(rows(page).filter({ hasText: "Nova" }).locator("img")).toHaveCount(0);
  await expect(rows(page).filter({ hasText: "Nova" }).getByText("N", { exact: true })).toHaveCSS("color", "rgb(244, 114, 182)");
  expect(mock.unhandled).toEqual([]);
});

test("set as default moves the badge; deleting the default asks who takes over", async ({ page }) => {
  const mock = await installChatApi(page, { personas: team() });
  await openPanel(page);
  await expect(rows(page)).toHaveCount(3);
  await expect(rows(page).first()).toContainText("Atlas");

  await rows(page).filter({ hasText: "Sage" }).getByRole("button", { name: "Set as default" }).click();
  await expect(rows(page).first()).toContainText("Sage");
  await expect(rows(page).first()).toContainText("Default");
  await expect(rows(page).filter({ hasText: "Atlas" })).not.toContainText("Default");
  expect(mock.personas.filter((p) => p.is_default).map((p) => p.name)).toEqual(["Sage"]);

  await panel(page).getByRole("button", { name: "Delete Sage" }).click();
  const confirm = page.getByRole("dialog", { name: "Delete Sage?" });
  await expect(confirm).toContainText("Sage is your default. Choose a new default:");
  await expect(confirm).toContainText("Its 3 conversations are kept and continue with the built-in Assistant.");
  // Preselected: the most recently used of the rest.
  await expect(confirm.getByRole("radio", { name: "Atlas" })).toBeChecked();
  await confirm.getByRole("radio", { name: "Nova" }).check();
  await confirm.getByRole("button", { name: "Delete Sage" }).click();
  await expect(confirm).toHaveCount(0);
  await expect(rows(page)).toHaveCount(2);
  await expect(rows(page).first()).toContainText("Nova");
  await expect(rows(page).first()).toContainText("Default");
  expect(mock.personas.find((p) => p.is_default)?.name).toBe("Nova");

  await panel(page).getByRole("button", { name: "Delete Atlas" }).click();
  const plain = page.getByRole("dialog", { name: "Delete Atlas?" });
  await expect(plain.getByRole("radio")).toHaveCount(0);
  await plain.getByRole("button", { name: "Delete Atlas" }).click();
  await expect(rows(page)).toHaveCount(1);
  expect(mock.unhandled).toEqual([]);
});

test("the list follows persona.updated, and a stale edit offers a reload", async ({ page }) => {
  const mock = await installChatApi(page, { personas: team() });
  await openPanel(page);
  await expect(rows(page)).toHaveCount(3);

  // Someone renames Nova in another tab.
  const nova = mock.personas.find((p) => p.name === "Nova")!;
  await panel(page).getByRole("button", { name: "Edit Nova" }).click();
  nova.name = "Nova Prime";
  nova.revision++;
  await pushPersonaUpdated(page, [nova.id]);
  await expect(rows(page).filter({ hasText: "Nova Prime" })).toHaveCount(1);

  await editor(page).getByLabel("Name").fill("Nova 2");
  await editor(page).getByRole("button", { name: "Save persona" }).click();
  const prompt = editor(page).getByRole("alert").filter({ hasText: "changed somewhere else" });
  await expect(prompt).toBeVisible();
  await prompt.getByRole("button", { name: "Reload" }).click();
  await expect(editor(page).getByLabel("Name")).toHaveValue("Nova Prime");
  await editor(page).getByRole("radio", { name: "Formal" }).click();
  await editor(page).getByRole("button", { name: "Save persona" }).click();
  await expect(editor(page)).toHaveCount(0);
  expect(nova).toMatchObject({ name: "Nova Prime", tone: "formal" });
  expect(mock.unhandled).toEqual([]);
});

test("abilities: Write code reveals repositories; they validate, save with the capabilities, and persist when code is off", async ({ page }) => {
  const mock = await installChatApi(page, { personas: team() });
  await openPanel(page);
  await panel(page).getByRole("button", { name: "Edit Nova" }).click();

  await expect(editor(page).getByLabel("Repositories")).toHaveCount(0);
  await editor(page).getByRole("switch", { name: "Write code" }).check();
  await expect(editor(page)).toContainText("Add at least one repository to work in.");

  const input = editor(page).getByLabel("Repositories", { exact: true }).and(page.locator("input"));
  await input.fill("https://user:token@github.com/acme/app");
  await input.press("Enter");
  await expect(editor(page).getByRole("alert")).toContainText("Connections & secrets");
  await input.fill("github.com/acme/app");
  await input.press("Enter");
  await expect(editor(page).getByRole("alert")).toContainText("Use https://");

  await input.fill("https://github.com/acme/app");
  await input.press("Enter");
  await expect(editor(page).getByTestId("persona-repo")).toHaveCount(1);
  await input.fill("https://github.com/acme/app");
  await input.press("Enter");
  await expect(editor(page).getByRole("alert")).toContainText("Already in the list");

  await editor(page).getByRole("switch", { name: "Generate images" }).check();
  await editor(page).getByRole("button", { name: "Save persona" }).click();
  await expect(editor(page)).toHaveCount(0);
  const nova = mock.personas.find((p) => p.name === "Nova")!;
  expect(nova.capabilities).toEqual({ code: true, images: true, personas: false, svg: false });
  expect(nova.repos).toEqual(["https://github.com/acme/app"]);
  const granted = rows(page).filter({ hasText: "Nova" }).getByTestId("persona-permissions");
  await expect(granted).toContainText("Write code");
  await expect(granted).toContainText("Generate images");
  await expect(rows(page).filter({ hasText: "Atlas" })).toContainText("No extra permissions");

  // Turning code off keeps the repositories and still sends them.
  await panel(page).getByRole("button", { name: "Edit Nova" }).click();
  await editor(page).getByRole("switch", { name: "Write code" }).uncheck();
  await editor(page).getByRole("button", { name: "Save persona" }).click();
  await expect(editor(page)).toHaveCount(0);
  expect(nova.capabilities.code).toBe(false);
  expect(nova.repos).toEqual(["https://github.com/acme/app"]);
  expect(mock.unhandled).toEqual([]);
});

test("a coding persona's new chat shows its repo and work branch in the header", async ({ page }) => {
  const repos = ["https://github.com/acme/app.git", "git@github.com:acme/api.git"];
  const mock = await installChatApi(page, {
    personas: [persona(1, { name: "Dev", is_default: true, capabilities: { code: true, images: true }, repos })],
  });
  await page.goto("/chat?project=WEB");
  await page.getByLabel("Repository for this conversation").selectOption(repos[1]);
  await page.getByLabel("Message").fill("fix the build");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page).toHaveURL(/\/chat\/[^/]+/);
  const chip = page.getByTestId("repo-chip").first();
  await expect(chip).toContainText("acme/api");
  await expect(chip).toContainText("vesper/dev-");
  await expect(chip).toHaveAttribute("title", repos[1]);
  await expect(page.getByTestId("image-hint")).toHaveCount(0);
  expect(mock.created[0]).toMatchObject({ persona_id: mock.personas[0].id, repo_url: repos[1] });
  expect(mock.unhandled).toEqual([]);
});
