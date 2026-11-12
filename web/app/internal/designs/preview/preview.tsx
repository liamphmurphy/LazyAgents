"use client";

import { useEffect, useState } from "react";
import { DesignSvg, loadDesignFonts } from "@/lib/designs/render";
import type { Design } from "@/lib/designs/schema";

type Input = { design: Design; page?: string; node?: string; w: number; h: number; assets: Record<string, string> };

/** Reads window.__preview (set by the spec before load), draws it once Inter has loaded, then sets window.__ready. */
export function Preview() {
  const [input, setInput] = useState<Input>();
  const [fonts, setFonts] = useState(false);
  useEffect(() => {
    // window.__preview only exists in the browser; reading it during render would mismatch the server HTML.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setInput((window as unknown as { __preview?: Input }).__preview);
    loadDesignFonts().then(() => setFonts(true));
  }, []);
  useEffect(() => {
    if (!input || !fonts) return;
    Promise.all([...document.images].map((i) => i.decode?.().catch(() => undefined))).then(() => {
      (window as unknown as { __ready: boolean }).__ready = true;
    });
  }, [input, fonts]);
  if (!input) return null;
  const page = input.design.pages.find((p) => p.id === input.page);
  return (
    <div id="shot" style={{ width: input.w, height: input.h }}>
      <style>{"#shot svg{display:block;width:100%;height:100%}body{margin:0}"}</style>
      <DesignSvg design={input.design} page={page} node={input.node} assetUrl={(a) => input.assets[a] ?? ""} />
    </div>
  );
}
