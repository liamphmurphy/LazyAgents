"use client";

import { useMemo, useSyncExternalStore } from "react";
import { VIZ_DARK, type VizPalette } from "@/lib/chat-svg";

function subscribe(onChange: () => void) {
  const attr = new MutationObserver(onChange);
  attr.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "data-palette", "style"] });
  const os = window.matchMedia("(prefers-color-scheme: light)");
  os.addEventListener("change", onChange);
  return () => { attr.disconnect(); os.removeEventListener("change", onChange); };
}

const server = JSON.stringify(VIZ_DARK);
function snapshot(): string {
  const css = getComputedStyle(document.documentElement);
  const color = (key: string, fallback: string) => css.getPropertyValue(`--viz-${key}`).trim() || fallback;
  return JSON.stringify({
    ...Object.fromEntries((["surface", "panel", "ink", "muted", "line", "grid"] as const).map((key) => [key, color(key, VIZ_DARK[key])])),
    series: VIZ_DARK.series.map((fallback, i) => color(String(i + 1), fallback)),
  });
}

/** Concrete colors for isolated SVG images, charts, Mermaid and code tokens. */
export function useVizPalette(): VizPalette {
  const value = useSyncExternalStore(subscribe, snapshot, () => server);
  return useMemo(() => JSON.parse(value) as VizPalette, [value]);
}
