import { expect, test, type Page } from "@playwright/test";
import { installChatApi, openEmptyChat, composerOption } from "./mock-chat-api";

const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });
const bubble = (page: Page) => page.getByTestId("snippet-bubble");
const chips = (page: Page) => page.getByTestId("snippet-chip");

const GO = `func retry(ctx context.Context, n int) error {
	for i := 0; i < n; i++ {
		if err := call(ctx); err == nil {
			return nil
		}
	}
	return errors.New("gave up")
}`;

// A paste the way the browser does it: the paste event (which the composer inspects), then the text lands.
async function paste(page: Page, text: string) {
  await composer(page).focus();
  await composer(page).evaluate((el, text) => {
    const data = new DataTransfer();
    data.setData("text/plain", text);
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, text);
  await page.keyboard.insertText(text);
}

async function fakeUploads(page: Page) {
  const names: string[] = [];
  await page.route("**/api/v1/chat-attachments", async (route) => {
    const name = /filename="([^"]+)"/.exec(route.request().postDataBuffer()?.toString("latin1") ?? "")?.[1] ?? "file";
    names.push(name);
    return route.fulfill({ status: 201, json: { id: `att-${names.length}`, filename: name, content_type: "text/plain", size_bytes: 10 } });
  });
  return names;
}

test("a snippet from the Code bubble waits as a chip and is sent as a fenced block after the text", async ({ page }) => {
  const mock = await installChatApi(page);
  await openEmptyChat(page);

  await (await composerOption(page, "code")).getByRole("button", { name: "Add code or Markdown" }).click();
  await expect(bubble(page)).toBeVisible();
  await bubble(page).getByTestId("snippet-input").fill(GO);
  // The language follows the text until it's picked by hand.
  await expect(bubble(page).getByTestId("snippet-lang")).toHaveValue("go");
  await expect(bubble(page).getByTestId("snippet-detected")).toBeVisible();
  await bubble(page).getByRole("button", { name: "Add to message" }).click();
  await expect(bubble(page)).toHaveCount(0);
  await expect(composer(page)).toBeFocused();
  await expect(chips(page)).toHaveCount(1);
  await expect(chips(page)).toHaveAttribute("data-lang", "go");
  await expect(chips(page)).toContainText("Go snippet");
  await expect(chips(page)).toContainText("8 lines");

  // A click on the chip reopens it; a language picked by hand sticks.
  await chips(page).getByRole("button", { name: "Edit Go snippet" }).click();
  await bubble(page).getByTestId("snippet-lang").selectOption("text");
  await expect(bubble(page).getByTestId("snippet-detected")).toHaveCount(0);
  await bubble(page).getByTestId("snippet-input").press("Control+End");
  await bubble(page).getByTestId("snippet-input").pressSequentially(" // done");
  await expect(bubble(page).getByTestId("snippet-lang")).toHaveValue("text");
  await bubble(page).getByRole("button", { name: "Save" }).click();
  await expect(chips(page)).toHaveAttribute("data-lang", "text");

  await composer(page).fill("Why does this never back off?");
  await composer(page).press("Enter");
  await expect(page.getByTestId("assistant-message").first()).toContainText("Echo: Why does this never back off?");
  expect(mock.posted.at(-1)?.body.body).toBe(`Why does this never back off?\n\n\`\`\`text\n${GO} // done\n\`\`\``);
  await expect(chips(page)).toHaveCount(0);
  // The sent message renders the block as code.
  await expect(page.getByTestId("user-message").getByTestId("code-fence")).toContainText("errors.New");
});

test("a message can be only a snippet, and Ctrl+Enter adds it", async ({ page }) => {
  const mock = await installChatApi(page);
  await openEmptyChat(page);
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toBeDisabled();

  await (await composerOption(page, "code")).getByRole("button", { name: "Add code or Markdown" }).click();
  await bubble(page).getByTestId("snippet-input").fill('{"retries": 5, "backoff": "exp"}');
  await expect(bubble(page).getByTestId("snippet-lang")).toHaveValue("json");
  await bubble(page).getByTestId("snippet-input").press("Control+Enter");
  await expect(chips(page)).toHaveAttribute("data-lang", "json");
  await expect(send).toBeEnabled();
  await send.click();
  await expect.poll(() => mock.posted.at(-1)?.body.body).toBe('```json\n{"retries": 5, "backoff": "exp"}\n```');
});

test("Markdown with its own fences is sent in a longer fence", async ({ page }) => {
  const mock = await installChatApi(page);
  await openEmptyChat(page);
  const md = "## Retry policy\n\n- Back off **exponentially**\n\n```yaml\nretries: 5\n```";
  await (await composerOption(page, "code")).getByRole("button", { name: "Add code or Markdown" }).click();
  await bubble(page).getByTestId("snippet-input").fill(md);
  await expect(bubble(page).getByTestId("snippet-lang")).toHaveValue("markdown");
  await bubble(page).getByRole("button", { name: "Add to message" }).click();
  await expect(chips(page)).toContainText("Markdown (raw)");
  await composer(page).fill("Review this");
  await composer(page).press("Enter");
  await expect.poll(() => mock.posted.at(-1)?.body.body).toBe(`Review this\n\n\`\`\`\`markdown\n${md}\n\`\`\`\``);
});

test("pasting code offers a code block, and ignoring the offer leaves the paste as typed", async ({ page }) => {
  await installChatApi(page);
  await openEmptyChat(page);
  const offer = page.getByTestId("paste-offer");

  await composer(page).fill("Why? ");
  await paste(page, GO);
  await expect(offer).toContainText("Looks like Go");
  await offer.getByRole("button", { name: "Code block" }).click();
  await expect(offer).toHaveCount(0);
  await expect(composer(page)).toHaveValue("Why? ");
  await expect(chips(page)).toHaveAttribute("data-lang", "go");

  // Prose isn't offered.
  await paste(page, "It failed again this morning.\nThe third attempt timed out.\nNothing in the logs.");
  await expect(offer).toHaveCount(0);

  // Keep as text leaves the paste where it landed.
  await composer(page).fill("");
  await paste(page, GO);
  await offer.getByRole("button", { name: "Keep as text" }).click();
  await expect(offer).toHaveCount(0);
  await expect(composer(page)).toHaveValue(GO);
  await expect(chips(page)).toHaveCount(1);
});

test("a very long paste is offered as a file, from the message box and from the bubble", async ({ page }) => {
  await installChatApi(page);
  const uploads = await fakeUploads(page);
  await openEmptyChat(page);
  const log = Array.from({ length: 500 }, (_, i) => `2026-11-13T09:14:${String(i % 60).padStart(2, "0")}Z INFO step ${i}`).join("\n");

  await paste(page, log);
  const offer = page.getByTestId("paste-offer");
  await expect(offer).toContainText("500 lines pasted");
  await offer.getByRole("button", { name: "Attach as file" }).click();
  await expect(composer(page)).toHaveValue("");
  await expect(page.getByTestId("attachment-chip")).toHaveCount(1);
  await expect(page.getByTestId("attachment-chip")).toHaveAttribute("data-status", "done");
  expect(uploads).toEqual(["pasted-log-1.txt"]);

  await (await composerOption(page, "code")).getByRole("button", { name: "Add code or Markdown" }).click();
  await bubble(page).getByTestId("snippet-input").fill(log);
  await expect(bubble(page).getByTestId("snippet-big")).toContainText("500 lines");
  await bubble(page).getByRole("button", { name: "Attach as file" }).click();
  await expect(page.getByTestId("attachment-chip")).toHaveCount(2);
  expect(uploads).toEqual(["pasted-log-1.txt", "pasted-log-2.txt"]);
  await expect(chips(page)).toHaveCount(0);
});
