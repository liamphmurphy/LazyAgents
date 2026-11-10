"use client";

import Link from "next/link";
import { useEffect, useRef } from "react";
import { useChatCanvas } from "@/lib/api/canvas-hooks";
import { useDoc } from "@/lib/api/docs-hooks";
import { settled, useLightTheme } from "@/lib/print-ready";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { Markdown } from "@/components/domain/markdown";
import { PrintMode } from "@/components/domain/print-mode";

export function PrintDoc({ id }: { id: string }) {
  const themed = useLightTheme();
  const doc = useDoc(id);
  const chatId = doc.data?.chat_id;
  const cv = useChatCanvas(chatId ?? "", chatId ? id : undefined);
  const c = cv.data;
  const error = doc.error ?? cv.error;
  const ref = useRef<HTMLElement>(null);
  const printed = useRef(false);

  // The PDF's default file name is the page title.
  useEffect(() => {
    if (c) document.title = c.title;
  }, [c]);

  // Open the print dialog once, when everything on the page has drawn.
  useEffect(() => {
    if (!themed || !c || printed.current || !ref.current) return;
    printed.current = true;
    let live = true;
    settled(ref.current).then(() => {
      if (live) window.print();
    });
    return () => {
      live = false;
      printed.current = false;
    };
  }, [themed, c]);

  if (error) {
    return (
      <div className="mx-auto max-w-[720px] p-8">
        <Banner tone="blocked" title="Couldn't load this document">{error.message}</Banner>
      </div>
    );
  }

  return (
    <div className="min-h-dvh bg-surface text-fg print:bg-transparent">
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-border-subtle bg-panel px-6 py-2 print:hidden">
        <Link href={`/apps/docs/${id}`} className="text-[13px] text-fg-secondary hover:text-fg">
          ← Back to document
        </Link>
        <span className="ml-auto text-xs text-fg-tertiary">Choose “Save as PDF” as the destination.</span>
        <Button variant="primary" icon="printer" disabled={!c} onClick={() => window.print()}>
          Print / Save as PDF
        </Button>
      </div>
      <article ref={ref} data-testid="print-doc" className="print-doc mx-auto flex w-full max-w-[760px] flex-col gap-5 px-8 py-10 print:max-w-none print:p-0">
        {!themed || !c ? (
          <div aria-busy="true" className="flex flex-col gap-4">
            <Skeleton className="h-7 w-2/3" />
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-5/6" />
          </div>
        ) : (
          <PrintMode.Provider value={true}>
            <Markdown anchors visualBlocks className="type-reading text-fg-2">{c.body}</Markdown>
          </PrintMode.Provider>
        )}
      </article>
    </div>
  );
}
