"use client";

import { HarnessFooter } from "@/components/shell/harness-footer";
import { Dialog } from "radix-ui";
import { usePathname } from "next/navigation";
import { useRef, useState, type MouseEvent, type PointerEvent, type ReactNode } from "react";
import { viewedChatId } from "@/lib/persona-dock";
import { SidebarSpaces } from "@/app/(app)/spaces/_components/spaces-nav";
import { ChatHistory, NewConversationButton } from "./chat-history";

/** How far left (CSS px) a swipe has to travel, and how much further than it moves up or down, to close the drawer. */
const SWIPE = 60;

/**
 * The compact chat's history (DESIGN.md §3 Responsive): ≡ in the chat header opens it from the left
 * over the chat. New chat, Search chats, the persona row, spaces, then recent chats, from the same
 * components the sidebar uses. A modal dialog, so it traps focus, closes on Escape and on a scrim
 * tap, and hands focus back to `trigger`. Opening anything from it, or swiping left, closes it.
 */
export function HistoryDrawer({ trigger }: { trigger: ReactNode }) {
  const [open, setOpen] = useState(false);
  const pathname = usePathname();
  const viewed = viewedChatId(pathname);
  const start = useRef<{ x: number; y: number } | null>(null);
  // A link (a chat, a search hit, a space) closes it, even one to the chat already open.
  const onClick = (e: MouseEvent) => {
    if ((e.target as Element).closest("a[href]")) setOpen(false);
  };
  const onPointerDown = (e: PointerEvent) => {
    start.current = e.pointerType === "mouse" ? null : { x: e.clientX, y: e.clientY };
  };
  const onPointerUp = (e: PointerEvent) => {
    const s = start.current;
    start.current = null;
    if (!s) return;
    const dx = e.clientX - s.x;
    if (dx < -SWIPE && -dx > Math.abs(e.clientY - s.y)) setOpen(false);
  };
  // A persona pick opens that persona's chat by router.push: a new chat id closes it too.
  const [shownFor, setShownFor] = useState(viewed);
  if (viewed !== shownFor) {
    setShownFor(viewed);
    if (open) setOpen(false);
  }
  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Trigger asChild>{trigger}</Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay data-testid="history-scrim" className="motion-scrim fixed inset-0 z-40 bg-scrim/60" />
        <Dialog.Content
          aria-describedby={undefined}
          onClick={onClick}
          onPointerDown={onPointerDown}
          onPointerUp={onPointerUp}
          onPointerCancel={() => (start.current = null)}
          className="motion-drawer-left fixed inset-y-0 left-0 z-50 flex w-[min(320px,85vw)] touch-pan-y flex-col gap-3 border-r border-border bg-sidebar pl-safe pt-safe pb-safe shadow-[var(--shadow-float)] outline-none"
        >
          <Dialog.Title className="sr-only">History</Dialog.Title>
          <div className="flex min-h-0 flex-1 flex-col gap-3 px-3 pt-3 pb-3">
            <div className="flex flex-none">
              <NewConversationButton id={viewed ?? undefined} />
            </div>
            <ChatHistory id={viewed ?? undefined} viewed={viewed} fill between={<div className="pt-1"><SidebarSpaces viewed={viewed} hideWhenEmpty /></div>} />
          </div>
          {/* The rail's harness characters, which a phone has no rail for; Appearance hides them here too. */}
          <div className="px-2.5">
            <HarnessFooter layout="row" />
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
