import { expect, test, type Page } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

const ATLAS = persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true });

const b64 = (o: object) => Buffer.from(JSON.stringify(o)).toString("base64url");
// exp 2026-11-15T00:00:00Z
const JWT = `${b64({ alg: "HS256" })}.${b64({ sub: "liam", exp: 1794700800 })}.c2lnbmF0dXJlLXRoYXQtaXMtc2VjcmV0`;

async function openChat(page: Page) {
  await page.goto("/chat");
  await page.getByRole("textbox", { name: "Message" }).fill("Pull last week's orders from the Acme API");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
}

test("a secret is added with its hosts, then given to a persona with request permissions", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS] });
  await page.goto("/customization?who=secrets");
  await page.getByRole("button", { name: "Add secret" }).click();
  await page.getByLabel("Name").fill("Acme API JWT");
  await expect(page.getByText("Personas see it as acme-api-jwt")).toBeVisible();
  await page.getByLabel("Value").fill(JWT);
  await expect(page.getByTestId("jwt-expiry")).toContainText("Looks like a JWT · expires nov 15, 2026");
  await expect(page.getByTestId("secret-preview")).toHaveText("→ Authorization: Bearer ‹acme-api-jwt›");
  await page.getByLabel("Allowed hosts").fill("*.acme.com");
  await page.getByLabel("Allowed hosts").press("Enter");
  await expect(page.getByText("no wildcards")).toBeVisible();
  await page.getByLabel("Allowed hosts").fill("https://api.acme.com/v1");
  await page.getByLabel("Allowed hosts").press("Enter");
  await page.getByRole("button", { name: "Save" }).click();

  expect(mock.apiSecretPosts[0]).toMatchObject({ name: "Acme API JWT", value: JWT, placement: "bearer", allowed_hosts: ["api.acme.com"] });
  const detail = page.getByTestId("secret-detail");
  await expect(detail.getByRole("heading", { name: "Acme API JWT" })).toBeVisible();
  // Once saved, the value never shows again.
  await expect(page.getByText(JWT)).toHaveCount(0);
  await detail.getByRole("switch", { name: "Atlas can use Acme API JWT" }).click();
  await expect.poll(() => (mock.apiSecrets[0].persona_ids as string[]).length).toBe(1);
  await detail.getByRole("radiogroup", { name: "Writes with Acme API JWT" }).getByRole("radio", { name: "Off" }).click();
  await expect.poll(() => mock.apiSecrets[0].write_policy).toBe("off");
  expect(mock.unhandled).toEqual([]);
});

test("a request with a secret waits in the chat; writes can only be allowed once", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS] });
  await openChat(page);
  mock.secretApprovals.push({
    id: "21111111-1111-4111-8111-111111111111", secret_id: "21111111-1111-4111-8111-111111111112", secret_name: "Acme API JWT", secret_handle: "acme-jwt",
    placement: "bearer", placement_key: "", method: "POST", url: "https://api.acme.com/v1/refunds", headers: { Accept: "application/json" }, body: '{"order":"ord_9f2"}', created_at: new Date().toISOString(),
  });
  const card = page.getByTestId("secret-approval");
  await expect(card).toContainText("wants to call an API with");
  await expect(card).toContainText("POST https://api.acme.com/v1/refunds");
  await expect(card).toContainText("Authorization: Bearer ‹acme-jwt›");
  await expect(card).toContainText("api.acme.com is an allowed host");
  await expect(card.getByRole("button", { name: "Always allow reads" })).toHaveCount(0);
  await expect(card.getByTestId("secret-approval-body")).toHaveText('{"order":"ord_9f2"}');
  await expect(page.getByTestId("secrets-chip")).toContainText("acme-jwt waiting");
  await card.getByRole("button", { name: "Allow once" }).click();
  await expect(card).toHaveCount(0);
  expect(mock.secretDecisions).toEqual(["allow"]);

  // Afterwards the header says it was used, and it can be turned off for this chat.
  mock.chatSecrets.push({ id: "21111111-1111-4111-8111-111111111112", name: "Acme API JWT", handle: "acme-jwt", calls: 1, methods: ["POST"], last_used_at: new Date().toISOString(), enabled: true });
  // Chat usage polls every 5 s.
  await expect(page.getByTestId("secrets-chip")).toContainText("acme-jwt used 1×", { timeout: 10_000 });
  await page.getByTestId("secrets-chip").click();
  await page.getByRole("button", { name: "Turn off for this chat" }).click();
  await expect.poll(() => mock.chatSecrets[0].enabled).toBe(false);
  expect(mock.unhandled).toEqual([]);
});

test("a persona asks for a secret; adding it starts from what it asked for", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS] });
  await openChat(page);
  mock.secretProposals.push({ id: "31111111-1111-4111-8111-111111111111", name: "Acme API JWT", host: "api.acme.com", placement: "header", placement_key: "X-Api-Key", reason: "Read-only access to orders.", created_at: new Date().toISOString() });
  const card = page.getByTestId("secret-proposal");
  // Proposals poll every 5 s, like connector proposals.
  await expect(card).toContainText("asks for a secret", { timeout: 10_000 });
  await expect(card.getByTestId("secret-proposal-host")).toHaveText("api.acme.com");
  await expect(card).toContainText("X-Api-Key header");
  await card.getByRole("button", { name: "Add secret…" }).click();
  const dialog = page.getByRole("dialog", { name: "Add secret" });
  await expect(dialog.getByLabel("Name", { exact: true })).toHaveValue("Acme API JWT");
  await expect(dialog.getByLabel("Header name")).toHaveValue("X-Api-Key");
  await expect(dialog.getByText("api.acme.com")).toBeVisible();
  await dialog.getByLabel("Value").fill("k-1234567890");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(card).toHaveCount(0);
  expect(mock.apiSecretPosts[0]).toMatchObject({ placement: "header", placement_key: "X-Api-Key", allowed_hosts: ["api.acme.com"], proposal_id: "31111111-1111-4111-8111-111111111111" });
  expect(mock.unhandled).toEqual([]);
});
