"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

/**
 * Which canvas is open in a chat, in the URL. With no choice made the newest
 * card is expanded; "none" means the person collapsed it.
 */
export function useCanvasPanel() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const raw = search.get("canvas");
  const view = search.get("canvas_view");
  const go = (canvas: string | null, full: boolean | "max") => {
    const q = new URLSearchParams(search.toString());
    if (canvas === null) q.delete("canvas");
    else q.set("canvas", canvas);
    if (full) q.set("canvas_view", full === "max" ? "max" : "full");
    else q.delete("canvas_view");
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };
  return {
    focus: raw && raw !== "none" ? raw : undefined,
    collapsed: raw === "none",
    /** Open as the canvas panel: in the desktop dock or the compact sheet, or maximised over the window. */
    fullscreen: view === "full" || view === "max",
    /** Taken over the whole window from the dock's Canvas tab. */
    maximized: view === "max",
    show: (id: string) => go(id, false),
    collapse: () => go("none", false),
    openFull: (id: string) => go(id, true),
    exitFull: () => go(raw, false),
    maximize: (id: string) => go(id, "max"),
    /** From over the window back to the dock's Canvas tab. */
    restore: () => go(raw, true),
  };
}
