"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

export type PlaygroundTab = "preview" | "api" | "logs";

const TABS: PlaygroundTab[] = ["preview", "api", "logs"];

/**
 * Whether a chat's playground is open beside it or over the whole window, and
 * which tab shows, in the URL so a reload keeps the person where they were.
 */
export function usePlaygroundPanel() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const view = search.get("playground");
  const rawTab = search.get("playground_tab") as PlaygroundTab | null;
  const go = (next: string | null, tab?: PlaygroundTab) => {
    const q = new URLSearchParams(search.toString());
    if (next === null) q.delete("playground");
    else q.set("playground", next);
    if (tab) q.set("playground_tab", tab);
    if (next === null) q.delete("playground_tab");
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };
  return {
    open: view === "open" || view === "full",
    fullscreen: view === "full",
    tab: rawTab && TABS.includes(rawTab) ? rawTab : "preview",
    show: () => go("open"),
    openFull: () => go("full"),
    exitFull: () => go("open"),
    close: () => go(null),
    setTab: (tab: PlaygroundTab) => go(view ?? "open", tab),
  };
}
