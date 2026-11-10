"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useChatCanvas } from "@/lib/api/canvas-hooks";
import { docPrintHref, downloadMarkdown, useDoc } from "@/lib/api/docs-hooks";
import { useChat } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { AppDrawer } from "@/components/domain/app-drawer";
import { CanvasView } from "@/components/domain/canvas-view";
import { Conversation } from "@/app/(app)/chat/_components/conversation";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/**
 * A document full page: the canvas with its versions, comments and editor, and
 * its chat in a drawer beside it, so asking for a change shows up in place.
 */
export function DocScreen({ id }: { id: string }) {
  const doc = useDoc(id);
  const router = useRouter();
  const chatId = doc.data?.chat_id;
  const chat = useChat(chatId).data;
  const live = chat?.state === "running";
  const current = useChatCanvas(chatId ?? "", chatId ? id : undefined, undefined, live).data;
  const [askOpen, setAskOpen] = useState(true);
  const title = current?.title ?? doc.data?.title;

  return (
    <div className="flex h-dvh flex-col">
      <TopBar
        crumbs={[{ label: "Apps" }, { label: "Docs", href: "/apps/docs" }, { label: title ?? "Document" }]}
        actions={
          doc.data && (
            <>
              <Button variant="secondary" icon="download" disabled={!current} onClick={() => current && downloadMarkdown(current.title, current.body)}>
                Markdown
              </Button>
              <Link href={docPrintHref(id)} target="_blank" className="inline-flex h-[30px] items-center gap-1.5 rounded-md border border-btn-secondary-border bg-btn-secondary px-3 text-[13px] text-fg hover:bg-hover">
                PDF
              </Link>
              <Button variant="primary" onClick={() => setAskOpen((o) => !o)}>
                ✦ Ask
              </Button>
            </>
          )
        }
      />
      {doc.isError ? (
        <div className="mx-auto w-full max-w-[720px] p-6">
          <Banner tone="blocked" title="Couldn’t open this document">{errorText(doc.error)}</Banner>
        </div>
      ) : !chatId ? (
        <div aria-busy="true" className="mx-auto flex w-full max-w-[820px] flex-col gap-3 p-6">
          <Skeleton className="h-7 w-1/2" />
          <Skeleton className="h-[320px]" />
        </div>
      ) : (
        <main data-testid="doc-page" className={cx("flex min-h-0 flex-1 flex-col bg-surface-sunken", askOpen && "md:pr-[440px]")}>
          <CanvasView chatId={chatId} canvasId={id} live={live} readOnly={!!chat?.archived} agent={(chat?.speaker ?? chat?.persona)?.name ?? "Assistant"} page />
        </main>
      )}
      {chatId && (
        <AppDrawer open={askOpen} onOpenChange={setAskOpen} modal={false} icon="edit" title="Ask about this document" subtitle="Ask for changes; they land in the document as a new version.">
          <div className="flex min-h-0 flex-1 flex-col">
            {/* A document keeps its one chat: /new starts another document instead. */}
            <Conversation key={chatId} id={chatId} onNew={() => router.push("/apps/docs")} />
          </div>
        </AppDrawer>
      )}
    </div>
  );
}
