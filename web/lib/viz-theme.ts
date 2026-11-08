"use client";

import { useSyncExternalStore } from "react";
import { VIZ_DARK, VIZ_LIGHT, type VizPalette } from "@/lib/chat-svg";

const LIGHT_OS = "(prefers-color-scheme: light)";

function isDark() {
  const theme = document.documentElement.getAttribute("data-theme");
  return theme ? theme === "dark" : !window.matchMedia(LIGHT_OS).matches;
}

function subscribe(onChange: () => void) {
  const attr = new MutationObserver(onChange);
  attr.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  const os = window.matchMedia(LIGHT_OS);
  os.addEventListener("change", onChange);
  return () => {
    attr.disconnect();
    os.removeEventListener("change", onChange);
  };
}

/** The palette for the theme showing now, following the toggle and the OS. */
export function useVizPalette(): VizPalette {
  return useSyncExternalStore(subscribe, isDark, () => true) ? VIZ_DARK : VIZ_LIGHT;
}
