"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useChatCanvas } from "@/lib/api/canvas-hooks";
import { useDeckRef } from "@/lib/api/slides-hooks";
import { cx } from "@/lib/cx";
import { assetHref, SlideSvg, useSlideFonts } from "@/lib/slides/render";
import { parseDeck, slideText, slideTitle } from "@/lib/slides/schema";
import { Spinner } from "@/components/ui/loading";

/**
 * The saved deck, full screen and in order (mockup 4/6). Keys: → ↓ Space
 * PgDn next, ← ↑ PgUp back, Home/End, a number then Enter jumps, F toggles
 * fullscreen, ? shows the keys, Esc leaves. Hidden slides are skipped and
 * slides change instantly.
 */
export function PresentDeck({ id }: { id: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const ref = useDeckRef(id);
  const chatId = ref.data?.chat_id ?? "";
  const c = useChatCanvas(chatId, chatId ? id : undefined).data;
  const fonts = useSlideFonts();
  const deck = useMemo(() => (c ? parseDeck(c.body) : null), [c]);
  const shown = useMemo(() => deck?.slides.filter((s) => !s.hidden) ?? [], [deck]);
  const [i, setI] = useState(() => Math.max(0, Number(search.get("slide") ?? 1) - 1));
  const [controls, setControls] = useState(true);
  const [help, setHelp] = useState(false);
  const [toast, setToast] = useState<string>();
  const typed = useRef("");
  const idle = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const root = useRef<HTMLDivElement>(null);
  const bar = useRef<HTMLDivElement>(null);
  const at = Math.min(i, Math.max(0, shown.length - 1));
  const flash = useCallback((text: string) => {
    setToast(text);
    setTimeout(() => setToast((t) => (t === text ? undefined : t)), 4000);
  }, []);
  const slide = shown[at];

  const exit = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined);
    const editorIndex = deck && slide ? deck.slides.findIndex((s) => s.id === slide.id) + 1 : 1;
    router.push(`/apps/slides/${id}?slide=${editorIndex}`);
  }, [deck, slide, id, router]);

  const arm = useCallback(() => {
    clearTimeout(idle.current);
    idle.current = setTimeout(() => {
      // Never hide while the bar has focus.
      if (!bar.current?.contains(document.activeElement)) setControls(false);
    }, 2000);
  }, []);
  const wake = useCallback(() => {
    setControls(true);
    arm();
  }, [arm]);

  const go = useCallback((n: number) => setI(Math.max(0, Math.min(shown.length - 1, n))), [shown.length]);

  useEffect(() => {
    root.current?.requestFullscreen?.().catch(() => flash("Press F for fullscreen"));
    arm();
    return () => clearTimeout(idle.current);
  }, [arm, flash]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      wake();
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const k = e.key;
      if (/^\d$/.test(k)) {
        typed.current = (typed.current + k).slice(-3);
        return;
      }
      if (k === "Enter" && typed.current) {
        e.preventDefault();
        go(Number(typed.current) - 1);
        typed.current = "";
        return;
      }
      typed.current = "";
      if (["ArrowRight", "ArrowDown", "PageDown", " "].includes(k)) {
        if (k === " " && (e.target as HTMLElement).tagName === "BUTTON") return;
        e.preventDefault();
        go(at + 1);
      } else if (["ArrowLeft", "ArrowUp", "PageUp"].includes(k)) {
        e.preventDefault();
        go(at - 1);
      } else if (k === "Home") go(0);
      else if (k === "End") go(shown.length - 1);
      else if (k === "?") setHelp((h) => !h);
      else if (k === "f" || k === "F") {
        if (document.fullscreenElement) void document.exitFullscreen();
        else void root.current?.requestFullscreen?.().catch(() => flash("Fullscreen isn’t available here"));
      } else if (k === "Escape") {
        if (help) setHelp(false);
        else exit();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [at, go, shown.length, help, exit, wake, flash]);

  // Swipes, and taps on the left or right third (touch screens).
  const touch = useRef<{ x: number; y: number } | null>(null);

  return (
    <div
      ref={root}
      className={cx("fixed inset-0 flex items-center justify-center bg-[#000]", !controls && "cursor-none")}
      onPointerMove={wake}
      onTouchStart={(e) => (touch.current = { x: e.touches[0].clientX, y: e.touches[0].clientY })}
      onTouchEnd={(e) => {
        const t = touch.current;
        touch.current = null;
        if (!t) return;
        const dx = e.changedTouches[0].clientX - t.x;
        if (Math.abs(dx) > 40) go(at + (dx < 0 ? 1 : -1));
        else {
          const third = window.innerWidth / 3;
          if (t.x > 2 * third) go(at + 1);
          else if (t.x < third) go(at - 1);
        }
      }}
    >
      {!deck ? (
        <p role="status" className="text-13 text-[#fff9]">{ref.isError ? "Couldn’t open this deck." : <><Spinner size={20} /><span className="sr-only">Loading</span></>}</p>
      ) : !slide ? (
        <p role="status" className="text-13 text-[#fff9]">Every slide in this deck is hidden.</p>
      ) : (
        <SlideSvg deck={deck} slide={slide} assetUrl={(a) => assetHref(id, a)} fontsReady={fonts} className="max-h-full max-w-full" label={`Slide ${at + 1} of ${shown.length}: ${slideText(slide) || "empty slide"}`} />
      )}
      <p aria-live="polite" className="sr-only">{slide ? `Slide ${at + 1} of ${shown.length}: ${slideTitle(slide)}` : ""}</p>
      <div
        ref={bar}
        className={cx("absolute bottom-5 left-1/2 flex -translate-x-1/2 items-center gap-1 rounded-xl border border-border-float bg-surface-raised/95 px-2 py-1.5 shadow-[var(--shadow-float)] transition-opacity", controls || help ? "opacity-100" : "pointer-events-none opacity-0")}
        onFocus={wake}
      >
        <button type="button" aria-label="Previous slide" className="grid size-[30px] place-items-center rounded-md text-18 text-fg hover:bg-hover disabled:opacity-40" disabled={at === 0} onClick={() => go(at - 1)}>‹</button>
        <span className="min-w-[56px] text-center font-mono text-13 text-fg">{shown.length ? `${at + 1} / ${shown.length}` : "–"}</span>
        <button type="button" aria-label="Next slide" className="grid size-[30px] place-items-center rounded-md text-18 text-fg hover:bg-hover disabled:opacity-40" disabled={at >= shown.length - 1} onClick={() => go(at + 1)}>›</button>
        <span aria-hidden className="mx-1 h-5 w-px bg-border-strong" />
        <button type="button" aria-label="Keyboard shortcuts" aria-expanded={help} className="grid size-[30px] place-items-center rounded-md text-13 font-semibold text-fg-secondary hover:bg-hover" onClick={() => setHelp((h) => !h)}>?</button>
        <button type="button" onClick={exit} className="ml-1 inline-flex h-[30px] items-center gap-2 rounded-md border border-btn-secondary-border bg-btn-secondary px-3 text-13 font-medium text-fg hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent">
          Exit <span className="font-mono text-11 text-fg-tertiary">esc</span>
        </button>
      </div>
      {help && (
        <div role="dialog" aria-label="Keyboard shortcuts" className="absolute right-6 top-6 w-[300px] rounded-[14px] border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)]">
          <p className="mb-2 text-14 font-semibold text-fg">Keyboard</p>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-13">
            {[["→ ↓ Space PgDn", "Next slide"], ["← ↑ PgUp", "Previous slide"], ["Home / End", "First / last"], ["3 then Enter", "Go to slide 3"], ["F", "Toggle fullscreen"], ["Esc", "Exit to the editor"], ["?", "Show or hide this"]].map(([k, d]) => (
              <div key={k} className="contents">
                <dt className="font-mono text-12 text-fg-2">{k}</dt>
                <dd className="text-right text-fg-secondary">{d}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}
      {toast && (
        <div role="status" className="absolute left-1/2 top-5 -translate-x-1/2 rounded-[10px] border border-border-float bg-surface-raised px-3 py-1.5 text-13 text-fg-2 shadow-[var(--shadow-float)]">
          {toast}
          <button type="button" className="ml-2 text-accent" onClick={() => setToast(undefined)} aria-label="Dismiss">×</button>
        </div>
      )}
    </div>
  );
}
