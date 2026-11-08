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
  const go = (canvas: string | null, full: boolean) => {
    const q = new URLSearchParams(search.toString());
    if (canvas === null) q.delete("canvas");
    else q.set("canvas", canvas);
    if (full) q.set("canvas_view", "full");
    else q.delete("canvas_view");
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };
  return {
    focus: raw && raw !== "none" ? raw : undefined,
    collapsed: raw === "none",
    fullscreen: search.get("canvas_view") === "full",
    show: (id: string) => go(id, false),
    collapse: () => go("none", false),
    openFull: (id: string) => go(id, true),
    exitFull: () => go(raw, false),
  };
}
