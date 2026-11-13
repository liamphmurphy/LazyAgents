import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Audio a coding chat renders: one card that follows its stages under the turn, then on the
// reply as a player with the waveform (or what went wrong), and its copy in the Library.

async function ask(page: Page, prompt: string) {
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill(prompt);
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
}

const stages = (card: ReturnType<Page["getByTestId"]>) => card.getByRole("list", { name: "Stages" }).getByRole("listitem");
const doneStages = (card: ReturnType<Page["getByTestId"]>) => card.getByRole("list", { name: "Stages" }).locator('li[data-state="done"]');

test("the card follows each stage", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  for (const [i, [state, done]] of ([["compose", 0], ["render", 1], ["check", 2]] as const).entries()) {
    const box = i === 0 ? page.getByRole("combobox", { name: "Message" }) : page.getByLabel("Message", { exact: true });
    await box.fill(`audio: ${state}`);
    await box.press("Enter");
    await expect(page.getByTestId("audio-card")).toHaveCount(i + 1);
    const card = page.getByTestId("audio-card").last();
    await expect(card).toHaveAttribute("data-stage", state);
    await expect(card).toContainText("Metal riff demo");
    await expect(doneStages(card)).toHaveCount(done);
    await expect(stages(card).nth(done)).toHaveAttribute("data-state", "active");
  }
});

test("rendering shows the renderer's progress and note", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await ask(page, "audio: render");
  const card = page.getByTestId("audio-card");
  await expect(card.getByTestId("audio-note")).toHaveText("Rendering MIDI with FluidSynth");
  await expect(card.getByTestId("audio-percent")).toHaveText("54%");
  await expect(card.getByRole("progressbar", { name: "Render progress" })).toHaveAttribute("aria-valuenow", "54");
  await expect(card.getByTestId("audio-method")).toHaveText("MIDI → FluidSynth (GeneralUser GS)");
});

test("the mix check lists numbers, not listening", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await ask(page, "audio: check");
  const card = page.getByTestId("audio-card");
  await expect(card.getByTestId("audio-note")).toContainText("you can’t listen from here");
  const checks = card.getByTestId("audio-checks").getByRole("listitem");
  await expect(checks).toHaveCount(2);
  await expect(checks.nth(0)).toContainText("✓ Peak stays under -1 dBFS");
  await expect(checks.nth(1)).toContainText("○ No silence longer than 1 s");
});

test("while the turn runs the card shows live with Stop", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 60_000 });
  await ask(page, "audio-live: render");
  const live = page.getByTestId("thinking").getByTestId("audio-card");
  await expect(live).toBeVisible();
  await expect(live.getByRole("button", { name: "Stop" })).toBeVisible();
});

test("a finished file plays in place and is saved to the Library", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await ask(page, "audio: done");
  const card = page.getByTestId("audio-card");
  await expect(card).toHaveAttribute("data-state", "done");
  await expect(doneStages(card)).toHaveCount(4);
  await expect(card).toContainText("metal-riff-demo.wav");
  await expect(card).toContainText("✓ Saved to Library");
  await expect(card.getByTestId("audio-meta")).toHaveText("MIDI → FluidSynth (GeneralUser GS) · 4.0 MB");
  // The waveform draws the 64 buckets and scrubs.
  await expect(card.getByTestId("waveform").locator("span")).toHaveCount(40);
  await expect(card.getByRole("slider", { name: "Seek" })).toBeVisible();
  await expect(card.getByTestId("audio-time")).toContainText("0:00 /");
  await expect(card.getByRole("button", { name: "Play" })).toBeVisible();
  await expect(card.getByRole("link", { name: "Download" })).toHaveAttribute("href", /\/audio\/[^/]+\/content\?download=true$/);
  await expect(card.getByRole("link", { name: "Open in Library" })).toHaveAttribute("href", /\/library\?tab=audio&item=/);
});

test("a failed render keeps what went wrong and offers a retry and the log", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await ask(page, "audio: failed");
  const card = page.getByTestId("audio-card");
  await expect(card).toHaveAttribute("data-state", "failed");
  await expect(card.getByRole("alert")).toContainText("Rendering failed");
  await expect(card.getByRole("alert")).toContainText("fluidsynth isn't installed");
  await expect(card).toContainText("Not saved");
  await card.getByRole("button", { name: "Show log" }).click();
  await expect(card.getByTestId("audio-log")).toContainText("fluidsynth: not found");
  await expect(card.getByRole("button", { name: "Retry render" })).toBeVisible();
});

test("a stopped reply says so", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await ask(page, "audio: stopped");
  await expect(page.getByTestId("audio-card").getByRole("alert")).toContainText("Stopped");
});
