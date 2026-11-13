"use client";

import { usePathname } from "next/navigation";
import { useEffect, useSyncExternalStore } from "react";
import { useMarkChatRead, usePersonaDock } from "@/lib/api/hooks";
import { viewedChatId } from "@/lib/persona-dock";

function subscribeVisible(fn: () => void) {
  document.addEventListener("visibilitychange", fn);
  return () => document.removeEventListener("visibilitychange", fn);
}

/** Opening a chat reads it, as does a reply arriving while it is visible. */
export function ViewedChatReader() {
  const pathname = usePathname();
  const dock = usePersonaDock();
  const visible = useSyncExternalStore(subscribeVisible, () => document.visibilityState === "visible", () => false);
  const viewed = visible ? viewedChatId(pathname) : null;
  const unreadViewed = dock.data?.personas.flatMap((e) => e.chats).find((c) => c.id === viewed && c.unread > 0);
  const { mutate } = useMarkChatRead();
  const seq = unreadViewed?.last_seq;
  useEffect(() => {
    if (viewed && seq) mutate({ id: viewed, seq });
  }, [viewed, seq, mutate]);
  return null;
}
