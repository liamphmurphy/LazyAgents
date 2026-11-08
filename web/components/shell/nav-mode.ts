"use client";

import { useEffect, useSyncExternalStore } from "react";
import { navMode, routeMode, type NavMode } from "@/lib/project-scope";

const MODE_KEY = "lazyagents.nav-mode";
const LAST_WORK_KEY = "lazyagents.nav-last-work";

const never = () => () => undefined;

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Not remembered: pinned pages fall back to Work. */
  }
}

/** The menu's mode and the last Work page; a page in one mode remembers it for the pinned pages. */
export function useNavMode(pathname: string): { mode: NavMode; lastWork: string | null } {
  const remembered = useSyncExternalStore(never, () => read(MODE_KEY) as NavMode | null, () => null);
  const lastWork = useSyncExternalStore(never, () => read(LAST_WORK_KEY), () => null);
  useEffect(() => {
    const mode = routeMode(pathname);
    if (!mode) return;
    write(MODE_KEY, mode);
    if (mode === "work") write(LAST_WORK_KEY, pathname);
  }, [pathname]);
  return { mode: navMode(pathname, remembered), lastWork };
}
