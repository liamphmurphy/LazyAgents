"use client";

import { useEffect } from "react";
import { recordRecent, type PaletteLink } from "@/lib/command-palette";

/** Adds the item a screen shows to ⌘K's Recent list once it has loaded (undefined until then). */
export function useRecordVisit(item: PaletteLink | undefined) {
  const key = item ? `${item.href}\n${item.title}` : "";
  useEffect(() => {
    if (item) recordRecent(window.localStorage, item);
    // The item is rebuilt every render; its href and title are what make a new visit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
}
