import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// The Welcome Wizard's harness step where runs use stored keys (the Docker
// backend): pasting a credential, a refusal, a verified card and the hints.
// The whole flow runs on the real stack in e2e-stack/welcome-wizard.spec.ts.

test("harness step: paste a key, Verify, and continue only once one verifies", async ({ page }) => {
  await installChatApi(page, { onboarded: false });
  const verifies: { harness: string; body: unknown }[] = [];
  await page.route("**/api/v1/secrets", (route) =>
    route.fulfill({ json: { key_configured: true, harness_keys_required: true, sign_in_available: false, known: [], secrets: [] } }));
  await page.route("**/api/v1/harness/*/verify", (route) => {
    const harness = route.request().url().split("/").at(-2)!;
    const body = route.request().postDataJSON();
    verifies.push({ harness, body });
    if (body?.credential === "sk-ant-api03-revoked") return route.fulfill({ json: { ok: false, models: [], error: "model discovery failed; check provider credentials and connectivity" } });
    return route.fulfill({ json: { ok: true, models: [{ id: "opus", label: "Opus" }, { id: "sonnet", label: "Sonnet" }, { id: "haiku", label: "Haiku" }] } });
  });
  await page.goto("/chat?new");
  const wizard = page.getByTestId("welcome-wizard");
  await wizard.getByRole("button", { name: "Let’s begin" }).click();
  await wizard.getByRole("button", { name: "Continue" }).click();

  const claude = wizard.getByRole("region", { name: "Claude Code" });
  const codex = wizard.getByRole("region", { name: "Codex" });
  await expect(codex.getByTestId("codex-images")).toHaveText("Images");
  await expect(codex.getByLabel("Codex credential")).toHaveAttribute("placeholder", "Paste auth.json");
  await codex.getByRole("radio", { name: "API key" }).click();
  await expect(codex.getByLabel("Codex credential")).toHaveAttribute("placeholder", "OPENAI_API_KEY");
  const next = wizard.getByRole("button", { name: "Continue" });
  await expect(next).toBeDisabled();
  await expect(wizard.getByTestId("wizard-hint")).toHaveText("Connect at least one to continue.");

  await claude.getByRole("radio", { name: "API key" }).click();
  await claude.getByLabel("Claude Code credential").fill("sk-ant-api03-revoked");
  await claude.getByRole("button", { name: "Verify Claude Code" }).click();
  await expect(claude.getByRole("alert")).toContainText("model discovery failed");
  await expect(next).toBeDisabled();
  await claude.getByLabel("Claude Code credential").fill("sk-ant-api03-good");
  await claude.getByRole("button", { name: "Verify Claude Code" }).click();
  await expect(claude.getByTestId("claude-code-models")).toHaveText("3 models");
  // The field is cleared once sent, so the key doesn't linger on screen.
  await expect(claude.getByLabel("Claude Code credential")).toHaveValue("");
  expect(verifies.at(-1)).toEqual({ harness: "claude-code", body: { kind: "api_key", credential: "sk-ant-api03-good" } });
  await expect(wizard.getByTestId("wizard-hint")).toHaveText("Image generation stays off until Codex or Antigravity is connected.");
  await expect(next).toBeEnabled();
});

// Antigravity has no subscription login to paste (Google's terms bar storing
// it), so its card is a Gemini key field alone, and a verified key also
// switches image generation on.
test("harness step: Antigravity takes only a Gemini API key", async ({ page }) => {
  await installChatApi(page, { onboarded: false });
  const verifies: { harness: string; body: unknown }[] = [];
  await page.route("**/api/v1/secrets", (route) =>
    route.fulfill({ json: { key_configured: true, harness_keys_required: true, sign_in_available: false, known: [], secrets: [] } }));
  await page.route("**/api/v1/harness/*/verify", (route) => {
    verifies.push({ harness: route.request().url().split("/").at(-2)!, body: route.request().postDataJSON() });
    return route.fulfill({ json: { ok: true, models: [{ id: "gemini-2.5-pro", label: "Gemini 2.5 Pro" }, { id: "gemini-2.5-flash", label: "Gemini 2.5 Flash" }] } });
  });
  await page.goto("/chat?new");
  const wizard = page.getByTestId("welcome-wizard");
  await wizard.getByRole("button", { name: "Let’s begin" }).click();
  await wizard.getByRole("button", { name: "Continue" }).click();

  const agy = wizard.getByRole("region", { name: "Antigravity" });
  await expect(agy.getByTestId("antigravity-images")).toHaveText("Images");
  await expect(agy.getByRole("radio")).toHaveCount(0);
  await expect(agy.getByLabel("Antigravity credential")).toHaveAttribute("placeholder", "GEMINI_API_KEY");
  await expect(agy).toContainText("never reads, copies or stores the Google sign-in");
  await agy.getByLabel("Antigravity credential").fill("AIzaSyGood");
  await agy.getByRole("button", { name: "Verify Antigravity" }).click();
  await expect(agy.getByTestId("antigravity-models")).toHaveText("2 models");
  expect(verifies.at(-1)).toEqual({ harness: "antigravity", body: { kind: "api_key", credential: "AIzaSyGood" } });
  await expect(wizard.getByTestId("wizard-hint")).toHaveText("");
  await expect(wizard.getByRole("button", { name: "Continue" })).toBeEnabled();
});
