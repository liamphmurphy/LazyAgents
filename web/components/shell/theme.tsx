"use client";

import { useSyncExternalStore } from "react";
import { Segmented } from "@/components/ui/controls";

export type ThemePref = "system" | "dark" | "light";
const KEY = "la-theme";

/**
 * Runs in <head> before first paint so a saved theme never flashes.
 * No attribute = follow the OS (tokens.css handles the media query).
 */
export const themeScript = `(function(){try{var t=localStorage.getItem("${KEY}");if(t==="dark"||t==="light")document.documentElement.setAttribute("data-theme",t)}catch(e){}})()`;

const listeners = new Set<() => void>();

function read(): ThemePref {
  try {
    const t = localStorage.getItem(KEY);
    return t === "dark" || t === "light" ? t : "system";
  } catch {
    return "system";
  }
}

function write(pref: ThemePref) {
  try {
    if (pref === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, pref);
  } catch {
    // Storage blocked: the choice still applies for this page view.
  }
  if (pref === "system") document.documentElement.removeAttribute("data-theme");
  else document.documentElement.setAttribute("data-theme", pref);
  listeners.forEach((l) => l());
}

export function useTheme(): [ThemePref, (p: ThemePref) => void] {
  const pref = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    read,
    () => "system" as ThemePref,
  );
  return [pref, write];
}

export function ThemeSwitch() {
  const [pref, setPref] = useTheme();
  return (
    <Segmented
      label="Theme"
      value={pref}
      onChange={setPref}
      options={[
        { value: "system", label: "System" },
        { value: "dark", label: "Dark" },
        { value: "light", label: "Light" },
      ]}
    />
  );
}
