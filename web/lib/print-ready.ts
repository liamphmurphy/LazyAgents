"use client";

import { useLayoutEffect, useSyncExternalStore } from "react";

// Shared by the print views: wait for a page to finish drawing, in the light theme, before printing.

/** Longest wait for diagrams, fonts and images before printing anyway. */
const READY_TIMEOUT_MS = 15_000;

function subscribeTheme(onChange: () => void) {
  const attr = new MutationObserver(onChange);
  attr.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  return () => attr.disconnect();
}

/**
 * Paper is white: print in the light theme whatever the app uses, and put it back after.
 * True once it applies, so visuals that read the theme once (Mermaid) draw light.
 */
export function useLightTheme() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    const prev = root.getAttribute("data-theme");
    root.setAttribute("data-theme", "light");
    return () => {
      if (prev) root.setAttribute("data-theme", prev);
      else root.removeAttribute("data-theme");
    };
  }, []);
  return useSyncExternalStore(subscribeTheme, () => document.documentElement.getAttribute("data-theme") === "light", () => false);
}

/** Resolves once Mermaid has drawn, fonts have loaded and every image has decoded. */
export async function settled(root: HTMLElement) {
  const deadline = Date.now() + READY_TIMEOUT_MS;
  const frame = () => new Promise((r) => requestAnimationFrame(r));
  while (Date.now() < deadline && root.querySelector('[data-testid="mermaid-loading"]')) {
    await new Promise((r) => setTimeout(r, 100));
  }
  await document.fonts.ready;
  await Promise.all(
    Array.from(root.querySelectorAll("img")).map((img) => img.decode().catch(() => undefined)),
  );
  await frame();
  await frame();
}
