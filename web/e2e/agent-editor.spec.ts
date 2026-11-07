import { expect, test, type Page } from "@playwright/test";
import { installAgentApi } from "./mock-agent-api";

const harness = (page: Page) => page.getByRole("radiogroup", { name: "Harness", exact: true });
const model = (page: Page) => page.getByLabel("Model", { exact: true });
const review = (page: Page) => page.getByRole("button", { name: /Review changes/ });
const save = (page: Page) => page.getByRole("dialog").getByRole("button", { name: /Save as v/ });

/** Saving is two steps: review the changes, then save from the dialog. */
async function saveDefinition(page: Page) {
  await review(page).click();
  await save(page).click();
}

async function pickOpenCodeByKeyboard(page: Page) {
  await harness(page).getByRole("radio", { name: "Claude Code" }).focus();
  await page.keyboard.press("ArrowRight"); // Codex
  await page.keyboard.press("ArrowRight"); // OpenCode
  await expect(harness(page).getByRole("radio", { name: "OpenCode" })).toBeChecked();
}

test("editor: choose OpenCode by keyboard, save, reload keeps it", async ({ page }) => {
  const mock = await installAgentApi(page);
  await page.goto("/agents/atlas?tab=settings");
  await expect(harness(page).getByRole("radio", { name: "OpenCode" })).toBeVisible();
  await expect(page.locator("#ag-models option[value='opus-5.5']")).toHaveCount(1);

  await pickOpenCodeByKeyboard(page);
  await expect(harness(page).getByRole("radio", { name: "OpenCode" })).toHaveText("OpenCode");
  await expect(page.getByText("Use provider/model, e.g. anthropic/claude-sonnet-5-5")).toBeVisible();
  await expect(page.locator("#ag-models option")).toHaveCount(3);
  await expect(page.locator("#ag-models option[value='anthropic/claude-sonnet-5-5']")).toHaveCount(1);
  await expect(page.locator("#ag-models option[value='opus-5.5']")).toHaveCount(0);

  await model(page).fill("anthropic/claude-sonnet-5-5");
  await saveDefinition(page);
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(review(page)).toBeDisabled();
  const spec = (mock.saved.definition.at(-1) as { spec: { harness: string; model: string } }).spec;
  expect(spec.harness).toBe("opencode");
  expect(spec.model).toBe("anthropic/claude-sonnet-5-5");

  await page.reload();
  await expect(harness(page).getByRole("radio", { name: "OpenCode" })).toBeChecked();
  await expect(model(page)).toHaveValue("anthropic/claude-sonnet-5-5");
  expect(mock.unhandled).toEqual([]);
});

test("editor: codex suggests codex models", async ({ page }) => {
  await installAgentApi(page);
  await page.goto("/agents/atlas?tab=settings");
  await harness(page).getByRole("radio", { name: "Codex" }).click();
  await expect(page.locator("#ag-models option[value='gpt-5-codex']")).toHaveCount(1);
  await expect(page.locator("#ag-models option[value='opus-5.5']")).toHaveCount(0);
  await expect(page.getByText("Use provider/model")).toHaveCount(0);
});

test("editor: fallback offers OpenCode and its default harness is not hardcoded", async ({ page }) => {
  await installAgentApi(page);
  await page.goto("/agents/atlas?tab=settings");
  await pickOpenCodeByKeyboard(page);
  await page.getByRole("button", { name: "Add fallback" }).click();
  const fb = page.getByRole("radiogroup", { name: "Fallback 1 harness" });
  await expect(fb.getByRole("radio", { name: "OpenCode" })).toBeVisible();
  // Defaults to the first harness other than the agent's own.
  await expect(fb.getByRole("radio", { name: "Claude Code" })).toBeChecked();
  await fb.getByRole("radio", { name: "OpenCode" }).click();
  await expect(fb.getByRole("radio", { name: "OpenCode" })).toBeChecked();
  const input = page.getByLabel("Fallback 1 model");
  await expect(input).toHaveAttribute("placeholder", "provider/model");
  await expect(page.locator("#ag-fb-models-0 option[value='openai/gpt-5']")).toHaveCount(1);
});

test("editor: 422 shows the error and keeps the draft", async ({ page }) => {
  const mock = await installAgentApi(page);
  await page.goto("/agents/atlas?tab=settings");
  await pickOpenCodeByKeyboard(page);
  await model(page).fill("sonnet-5.5");
  await saveDefinition(page);
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText("must be provider-qualified");
  expect(mock.saved.definition).toHaveLength(1);
  await expect(save(page)).toBeEnabled();
  await page.getByRole("button", { name: "Back to editing" }).last().click();
  await expect(model(page)).toHaveValue("sonnet-5.5");
  await expect(harness(page).getByRole("radio", { name: "OpenCode" })).toBeChecked();
  await expect(page.getByText(/\d+ unsaved changes?/)).toBeVisible();
  await expect(review(page)).toBeEnabled();
});

test("override dialog: offers OpenCode, saves it, reload shows it", async ({ page }) => {
  const mock = await installAgentApi(page);
  await page.goto("/agents/atlas?tab=settings");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await page.getByRole("menuitem", { name: "Web app" }).click();
  const dialog = page.getByRole("dialog");
  const radio = dialog.getByRole("radiogroup", { name: "Harness" }).getByRole("radio", { name: "OpenCode" });
  await expect(radio).toBeVisible();
  await radio.click();
  await expect(radio).toBeChecked();
  await dialog.getByLabel("Model", { exact: true }).fill("openai/gpt-5");
  await dialog.getByRole("button", { name: "Save override" }).click();
  await expect(dialog).toBeHidden();
  const body = mock.saved.overrides.at(-1)!.body;
  expect(body.harness).toBe("opencode");
  expect(body.model).toBe("openai/gpt-5");

  await page.reload();
  const panel = page.getByRole("complementary", { name: "Overrides and versions" });
  await expect(panel.getByText("harness = opencode")).toBeVisible();
  await panel.getByRole("button", { name: /Web app \d+ change/ }).click();
  await expect(page.getByRole("dialog").getByRole("radiogroup", { name: "Harness" }).getByRole("radio", { name: "OpenCode" })).toBeChecked();
});

test("editor saves selective memory and an explicit independent-review policy", async ({ page }) => {
  const mock = await installAgentApi(page);
  await page.goto("/agents/atlas?tab=settings");
  await expect(page.getByRole("radio", { name: "Selective", exact: true })).toBeChecked();
  await expect(page.getByLabel("Token budget", { exact: true })).toHaveValue("1500");
  await page.getByRole("switch", { name: "Require independent review before Done" }).click();
  // Commit the displayed memory defaults into this new definition explicitly.
  await page.getByRole("radio", { name: "Linked", exact: true }).click();
  await page.getByRole("radio", { name: "Selective", exact: true }).click();
  await saveDefinition(page);
  await expect(review(page)).toBeDisabled();
  const spec = (mock.saved.definition.at(-1) as { spec: { review_required: boolean; memory: { inject: string; max_tokens: number } } }).spec;
  expect(spec.review_required).toBe(true);
  expect(spec.memory).toMatchObject({ inject: "selective", max_tokens: 1500 });
  await page.reload();
  await expect(page.getByRole("switch", { name: "Require independent review before Done" })).toBeChecked();
  await expect(page.getByRole("radio", { name: "Selective", exact: true })).toBeChecked();
});
