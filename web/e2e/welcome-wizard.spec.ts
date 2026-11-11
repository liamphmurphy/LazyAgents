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
  await expect(codex).toContainText("Currently required for native AI image generation.");
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
  await expect(wizard.getByTestId("wizard-hint")).toHaveText("Image generation stays off until Codex is connected.");
  await expect(next).toBeEnabled();
});
