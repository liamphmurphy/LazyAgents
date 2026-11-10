"use client";

import Link from "next/link";
import { DropdownMenu } from "radix-ui";
import { useState } from "react";
import { generateHTML } from "@tiptap/core";
import { docPrintHref, downloadFile, useSaveDocToLibrary } from "@/lib/api/docs-hooks";
import { docExtensions, parseDoc } from "@/lib/doc-markdown";
import { toDocx } from "@/lib/doc-docx";
import { floatPanel } from "@/components/domain/pickers";
import { Button } from "@/components/ui/button";

const item = "flex cursor-pointer select-none flex-col rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";
const detail = "text-[11px] text-fg-tertiary";

type Notice = { tone: "ok" | "error"; text: string; href?: string };

/**
 * Every way a document leaves Vesper. `current` saves pending edits first and
 * returns the text as saved, so an export never misses the last few words.
 */
export function ExportMenu({ id, title, current }: { id: string; title: string; current: () => Promise<string> }) {
  const library = useSaveDocToLibrary(id);
  const [notice, setNotice] = useState<Notice>();
  const say = (n: Notice) => {
    setNotice(n);
    setTimeout(() => setNotice((x) => (x === n ? undefined : x)), 4000);
  };
  const attempt = (what: string, fn: () => Promise<Notice | void>) => () =>
    void fn()
      .then((n) => n && say(n))
      .catch((e: unknown) => say({ tone: "error", text: `${what} failed: ${e instanceof Error ? e.message : "try again"}` }));

  const word = attempt("Word export", async () => {
    const [md, docx] = await Promise.all([current(), import("docx")]);
    downloadFile(title, "docx", await docx.Packer.toBlob(toDocx(docx, parseDoc(md), title)));
  });
  const markdown = attempt("Markdown export", async () => downloadFile(title, "md", new Blob([await current()], { type: "text/markdown;charset=utf-8" })));
  const pdf = attempt("PDF export", async () => {
    await current();
    window.open(docPrintHref(id), "_blank", "noopener");
  });
  // Rich text pastes into mail, Google Docs or Slack with its formatting; the Markdown goes along as plain text.
  const copy = attempt("Copy", async () => {
    const md = await current();
    const html = generateHTML(parseDoc(md), docExtensions());
    await navigator.clipboard.write([new ClipboardItem({ "text/html": new Blob([html], { type: "text/html" }), "text/plain": new Blob([md], { type: "text/plain" }) })]);
    return { tone: "ok", text: "Copied with formatting" };
  });
  const save = attempt("Saving to the Library", async () => {
    await current();
    await library.mutateAsync();
    return { tone: "ok", text: "Saved to your Library", href: "/library" };
  });

  return (
    <div className="flex items-center gap-2">
      {notice && (
        <span role={notice.tone === "error" ? "alert" : "status"} className={notice.tone === "error" ? "text-[12px] text-pill-blocked-fg" : "text-[12px] text-fg-secondary"}>
          {notice.text}
          {notice.href && (
            <>
              {" · "}
              <Link href={notice.href} className="text-accent hover:underline">Open</Link>
            </>
          )}
        </span>
      )}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <Button variant="secondary" icon="download">Export</Button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={6} className={`${floatPanel} z-50 w-[250px]`}>
            <DropdownMenu.Item className={item} onSelect={pdf}>PDF<span className={detail}>Opens the print dialog</span></DropdownMenu.Item>
            <DropdownMenu.Item className={item} onSelect={word}>Word (.docx)<span className={detail}>Opens in Word, Pages or Google Docs</span></DropdownMenu.Item>
            <DropdownMenu.Item className={item} onSelect={markdown}>Markdown (.md)<span className={detail}>The document as written</span></DropdownMenu.Item>
            <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
            <DropdownMenu.Item className={item} onSelect={copy}>Copy as rich text<span className={detail}>Paste with formatting anywhere</span></DropdownMenu.Item>
            <DropdownMenu.Item className={item} disabled={library.isPending} onSelect={save}>Save to Library<span className={detail}>Keeps a copy as it is now</span></DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  );
}
