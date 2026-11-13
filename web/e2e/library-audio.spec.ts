import { expect, test, type Page } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installBoardApi } from "./mock-board-api";
import { mockWav } from "./mock-chat-api";

type Item = components["schemas"]["LibraryItem"];

// The Library's Audio tab: tiles with their waveform, a player dialog that scrubs, loops and
// renames, uploads, and the empty state.

const peaks = Array.from({ length: 64 }, (_, i) => Math.round(255 * (0.2 + 0.8 * Math.abs(Math.sin(i / 6)))));

const audio = (id: string, over: Partial<Item> = {}): Item => ({
  id, kind: "audio", title: "Metal riff demo", filename: "metal-riff-demo.wav", content_type: "audio/wav", size_bytes: 4_200_000, duration_ms: 24000, peaks,
  method: "MIDI → FluidSynth (GeneralUser GS)", source: "generated", persona_name: "Hackerman", chat_id: "chat-1", chat_title: "Heavy metal music", created_at: new Date().toISOString(), ...over,
});

async function installLibrary(page: Page, items: Item[]) {
  await installBoardApi(page);
  const seen = { renamed: [] as unknown[], uploaded: [] as string[], deleted: [] as string[] };
  await page.route("**/api/v1/library**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const m = url.pathname.match(/\/library\/([^/]+)(\/content)?$/);
    if (req.method() === "PATCH" && m) {
      const body = req.postDataJSON() as { title: string };
      seen.renamed.push(body);
      const it = items.find((i) => i.id === m[1])!;
      it.title = body.title;
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(it) });
    }
    if (req.method() === "DELETE" && m) {
      seen.deleted.push(m[1]);
      items.splice(items.findIndex((i) => i.id === m[1]), 1);
      return route.fulfill({ status: 204 });
    }
    if (req.method() === "POST" && url.pathname.endsWith("/library")) {
      const name = /filename="([^"]+)"/.exec(req.postData() ?? "")?.[1] ?? "";
      seen.uploaded.push(name);
      const it = audio("up-1", { title: name.replace(/\.\w+$/, ""), filename: name, source: "upload", method: undefined, chat_id: undefined, persona_name: undefined });
      items.unshift(it);
      return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(it) });
    }
    // Long enough that a slow test never reaches the end of the clip.
    if (m?.[2]) return route.fulfill({ status: 200, contentType: "audio/wav", body: mockWav(30) });
    const kind = url.searchParams.get("kind");
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: items.filter((i) => !kind || i.kind === kind), has_more: false }) });
  });
  return seen;
}

test("the Audio tab lists tiles with their waveform, length and source", async ({ page }) => {
  await installLibrary(page, [audio("a-1"), audio("a-2", { title: "Voice memo", source: "upload", method: undefined, duration_ms: 151000, chat_id: undefined, peaks: undefined })]);
  await page.goto("/library");
  await page.getByRole("tab", { name: "Audio" }).click();
  const tiles = page.getByTestId("library-item");
  await expect(tiles).toHaveCount(2);
  await expect(tiles.nth(0)).toContainText("0:24");
  await expect(tiles.nth(0)).toContainText("Hackerman");
  await expect(tiles.nth(0).getByRole("link", { name: "Heavy metal music" })).toHaveAttribute("href", "/chat/chat-1");
  await expect(tiles.nth(0).getByTestId("waveform").locator("span")).toHaveCount(32);
  await expect(tiles.nth(1)).toContainText("2:31");
  await expect(tiles.nth(1)).toContainText("Uploaded by you");
  // No peaks (a format the server couldn't decode) draws a flat line, not nothing.
  const flat = await tiles.nth(1).getByTestId("waveform").locator("span").evaluateAll((els) => els.map((e) => (e as HTMLElement).style.height));
  expect(new Set(flat)).toEqual(new Set(["8%"]));
});

test("a tile opens the player and starts it: scrub, speed, loop, details and download", async ({ page }) => {
  await installLibrary(page, [audio("a-1")]);
  await page.goto("/library?tab=audio");
  await page.getByRole("button", { name: "Play Metal riff demo" }).click();
  const dialog = page.getByRole("dialog", { name: "Metal riff demo" });
  const player = dialog.getByTestId("audio-player");
  // Clicking the tile is asking to hear it.
  await expect.poll(() => dialog.locator("audio").evaluate((a: HTMLAudioElement) => !a.paused)).toBe(true);
  await player.getByRole("button", { name: "Pause" }).click();
  await expect(player.getByTestId("waveform").locator("span")).toHaveCount(64);
  await expect(player.getByTestId("audio-time")).toContainText("0:");
  await expect(dialog.getByTestId("audio-made-with")).toHaveText("MIDI → FluidSynth (GeneralUser GS)");
  await expect(dialog.getByRole("link", { name: "Download" })).toHaveAttribute("href", "/api/v1/library/a-1/content?download=true");
  await expect(dialog.getByRole("link", { name: "Open chat" })).toHaveAttribute("href", "/chat/chat-1");

  await expect(player.getByRole("button", { name: "Speed 1×" })).toBeVisible();
  await player.getByRole("button", { name: "Speed 1×" }).click();
  await expect(player.getByRole("button", { name: "Speed 1.25×" })).toBeVisible();
  await expect(player.getByRole("button", { name: "Loop" })).toHaveAttribute("aria-pressed", "false");
  await player.getByRole("button", { name: "Loop" }).click();
  await expect(player.getByRole("button", { name: "Loop" })).toHaveAttribute("aria-pressed", "true");

  // Arrow keys on the waveform move the position by 5% of the clip.
  const seek = player.getByRole("slider", { name: "Seek" });
  // The file's own length (30 s here) replaces the planned one once its metadata loads.
  await expect(player.getByTestId("audio-time")).toContainText("/ 0:30");
  // A mocked response has no byte ranges, so the browser can't seek until the whole clip has arrived.
  await expect.poll(() => dialog.locator("audio").evaluate((a: HTMLAudioElement) => a.seekable.length > 0 && a.seekable.end(0) >= a.duration - 0.5)).toBe(true);
  await seek.focus();
  const before = Number(await seek.getAttribute("aria-valuenow"));
  await seek.press("ArrowRight");
  await expect.poll(async () => Number(await seek.getAttribute("aria-valuenow"))).toBeGreaterThanOrEqual(before + 4);
});

test("a deep link waits for Play, and closing the player stops the clip", async ({ page }) => {
  await installLibrary(page, [audio("a-1")]);
  await page.goto("/library?tab=audio&item=a-1");
  const dialog = page.getByRole("dialog", { name: "Metal riff demo" });
  const el = dialog.locator("audio");
  await expect(el).toHaveCount(1);
  expect(await el.evaluate((a: HTMLAudioElement) => a.paused)).toBe(true);
  await dialog.getByRole("button", { name: "Play" }).click();
  await expect.poll(() => el.evaluate((a: HTMLAudioElement) => !a.paused)).toBe(true);
  await dialog.getByRole("button", { name: "Pause" }).click();
  await expect.poll(() => el.evaluate((a: HTMLAudioElement) => a.paused)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(page.locator("audio")).toHaveCount(0);
});

test("a title is renamed in place", async ({ page }) => {
  const seen = await installLibrary(page, [audio("a-1")]);
  await page.goto("/library?tab=audio&item=a-1");
  const dialog = page.getByRole("dialog", { name: "Metal riff demo" });
  await expect(dialog.getByRole("button", { name: "Rename" })).toBeDisabled();
  await dialog.getByRole("textbox", { name: "Title" }).fill("Riff in E minor");
  await dialog.getByRole("button", { name: "Rename" }).click();
  await expect.poll(() => seen.renamed).toEqual([{ title: "Riff in E minor" }]);
  await expect(page.getByRole("dialog", { name: "Riff in E minor" })).toBeVisible();
});

test("removing an audio file asks first", async ({ page }) => {
  const seen = await installLibrary(page, [audio("a-1")]);
  await page.goto("/library?tab=audio&item=a-1");
  const dialog = page.getByRole("dialog", { name: "Metal riff demo" });
  let asked = "";
  page.once("dialog", (d) => {
    asked = d.message();
    return d.accept();
  });
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(page.getByTestId("library-item")).toHaveCount(0);
  expect(asked).toContain("audio file");
  expect(seen.deleted).toEqual(["a-1"]);
});

test("uploading an audio file adds it, and an executable is refused before sending", async ({ page }) => {
  const seen = await installLibrary(page, []);
  await page.goto("/library?tab=audio");
  await expect(page.getByRole("heading", { name: "Nothing here yet" })).toBeVisible();
  await expect(page.getByText("MP3, WAV, OGG or M4A file, like a voice memo")).toBeVisible();
  const input = page.getByLabel("File to upload");
  await input.setInputFiles({ name: "setup.exe", mimeType: "application/octet-stream", buffer: Buffer.from("x") });
  await expect(page.getByText("setup.exe isn't kept: executables and disk images aren't allowed.")).toBeVisible();
  expect(seen.uploaded).toEqual([]);
  await input.setInputFiles({ name: "Voice memo.wav", mimeType: "", buffer: mockWav() });
  await expect(page.getByTestId("library-item")).toHaveCount(1);
  await expect(page.getByTestId("library-item")).toContainText("Voice memo");
  expect(seen.uploaded).toEqual(["Voice memo.wav"]);
});
