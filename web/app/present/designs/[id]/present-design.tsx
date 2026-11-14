"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useDesign, useDesignVersion } from "@/lib/api/designs-hooks";
import { cx } from "@/lib/cx";
import { makeLayout, type LaidPage } from "@/lib/designs/layout";
import { follow, goBack, hotspotsOf, play, restart, screens, startScreen, transitionFrames, type Player } from "@/lib/designs/prototype";
import { designAssetHref, LaidNodeSvg, useDesignFonts } from "@/lib/designs/render";
import { normalize, type Design, type Link } from "@/lib/designs/schema";
import { Spinner } from "@/components/ui/loading";

type Move = { from: string; transition: Link["transition"]; duration: number; back?: boolean };

/** Plays a link's transition backwards when going back: a slide returns the way it came. */
const reverse = (t: Link["transition"]): Link["transition"] =>
  t === "slide-left" ? "slide-right" : t === "slide-right" ? "slide-left" : t === "slide-up" ? "slide-down" : t === "slide-down" ? "slide-up" : t;

/** The move to animate, or undefined when the change is instant (no frames, no duration, reduced motion). */
function moving(from: string, transition: Link["transition"], duration = 300, back?: boolean): Move | undefined {
  if (!transitionFrames(transition) || !duration) return undefined;
  if (typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return undefined;
  return { from, transition, duration, back };
}

/** One frame, fitted into the screen, with its hotspots on top. */
function FrameView({ design, laid, frame, hotspots, hints, onLink, assetUrl, label }: { design: Design; laid: LaidPage; frame: string; hotspots: Link[]; hints: boolean; onLink?: (l: Link) => void; assetUrl: (a: string) => string; label: string }) {
  const node = laid.nodes.find((n) => n.id === frame);
  const fb = laid.boxes.get(frame);
  const [size, setSize] = useState({ w: 0, h: 0 });
  const wrap = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setSize({ w: el.clientWidth, h: el.clientHeight }));
    ro.observe(el);
    setSize({ w: el.clientWidth, h: el.clientHeight });
    return () => ro.disconnect();
  }, []);
  if (!node || !fb) return null;
  const k = size.w && size.h ? Math.min(size.w / fb.w, size.h / fb.h) : 0;
  return (
    <div ref={wrap} className="absolute inset-0 flex items-center justify-center">
      {k > 0 && (
        <div className="relative" style={{ width: fb.w * k, height: fb.h * k }} data-frame={frame}>
          <svg viewBox={`${fb.x} ${fb.y} ${fb.w} ${fb.h}`} width="100%" height="100%" role="img" aria-label={label} className="block">
            <LaidNodeSvg ops={node.ops} uid={`pr${frame}`} assetUrl={assetUrl} />
          </svg>
          {onLink &&
            // After-delay links have no hotspot: they fire on their own.
            hotspots.filter((l) => l.trigger !== "after-delay").map((l) => {
              const hb = laid.boxes.get(l.from);
              if (!hb) return null;
              const name = design.pages.flatMap((p) => p.children).find((n) => n.id === l.to)?.name ?? "the next frame";
              return (
                <button
                  key={l.id}
                  type="button"
                  data-hotspot={l.id}
                  aria-label={`Go to ${name}`}
                  className={cx("absolute rounded-[2px] outline-none transition-colors focus-visible:ring-2 focus-visible:ring-accent", hints ? "bg-accent/25 ring-1 ring-accent" : "bg-transparent")}
                  style={{ left: (hb.x - fb.x) * k, top: (hb.y - fb.y) * k, width: hb.w * k, height: hb.h * k, cursor: l.trigger === "click" ? "pointer" : "default" }}
                  onClick={(e) => {
                    e.stopPropagation();
                    if (l.trigger === "click") onLink(l);
                  }}
                  onPointerEnter={() => l.trigger === "hover" && onLink(l)}
                  tabIndex={l.trigger === "click" ? 0 : -1}
                />
              );
            })}
        </div>
      )}
    </div>
  );
}

/**
 * The saved design's prototype, full screen (the Slides Present shell):
 * it starts at ?start (a frame, or a layer in one) or the page's first frame,
 * follows hotspots with each link's transition and skips broken links.
 * Keys: ← Backspace back, R restart, H shows hotspots, F fullscreen, ? the
 * keys, Esc leaves.
 */
export function PresentDesign({ id }: { id: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const ref = useDesign(id);
  const head = useDesignVersion(id, ref.data?.seq);
  useDesignFonts();
  const assetUrl = useCallback((a: string) => designAssetHref(id, a), [id]);
  const { design, error } = useMemo((): { design?: Design; error?: string } => {
    if (!head.data) return {};
    try {
      return { design: normalize(head.data.body).design };
    } catch (e) {
      return { error: e instanceof Error ? e.message : "This design can’t be opened." };
    }
  }, [head.data]);
  // Each page is laid out once, when Present first reaches one of its frames.
  const lay = useMemo(() => (design ? { design, layout: makeLayout(design), pages: new Map<string, LaidPage>() } : undefined), [design]);
  const laidOf = (frame: string): LaidPage | undefined => {
    const page = lay?.design.pages.find((p) => p.children.some((n) => n.id === frame));
    if (!lay || !page) return undefined;
    let l = lay.pages.get(page.id);
    if (!l) lay.pages.set(page.id, (l = lay.layout.layoutPage(page)));
    return l;
  };

  const start = useMemo(() => (design ? startScreen(design, search.get("start"), search.get("page")) : undefined), [design, search]);
  const [player, setPlayer] = useState<Player>();
  const [move, setMove] = useState<Move>();
  /** Counts frames shown (every follow, back and restart), so after-delay timers restart with each visit. */
  const [visit, setVisit] = useState(0);
  const p = player && design && screens(design).some((s) => s.id === player.at) ? player : start ? play(start.id) : undefined;
  const at = p?.at;

  const [controls, setControls] = useState(true);
  const [help, setHelp] = useState(false);
  const [hints, setHints] = useState(false);
  const [toast, setToast] = useState<string>();
  const root = useRef<HTMLDivElement>(null);
  const bar = useRef<HTMLDivElement>(null);
  const idle = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const hintTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const flash = useCallback((text: string) => {
    setToast(text);
    setTimeout(() => setToast((t) => (t === text ? undefined : t)), 4000);
  }, []);
  const arm = useCallback(() => {
    clearTimeout(idle.current);
    idle.current = setTimeout(() => {
      if (!bar.current?.contains(document.activeElement)) setControls(false);
    }, 2000);
  }, []);
  const wake = useCallback(() => {
    setControls(true);
    arm();
  }, [arm]);

  const exit = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined);
    router.push(`/apps/designs/${id}`);
  }, [id, router]);

  const go = useCallback(
    (l: Link) => {
      if (!p) return;
      const next = follow(p, l.to);
      if (next === p) return;
      setMove(moving(p.at, l.transition, l.duration_ms));
      setPlayer(next);
      setVisit((v) => v + 1);
    },
    [p],
  );
  const back = useCallback(() => {
    if (!p || !design) return;
    const prev = goBack(p);
    if (prev === p) return;
    // The link that brought us here sets how we go back.
    const came = design.links.find((l) => l.to === p.at && hotspotsOf(design, prev.at).includes(l));
    setMove(came ? moving(p.at, reverse(came.transition), came.duration_ms, true) : undefined);
    setPlayer(prev);
    setVisit((v) => v + 1);
  }, [p, design]);
  const again = useCallback(() => {
    if (!p) return;
    setMove(undefined);
    setPlayer(restart(p));
    setVisit((v) => v + 1);
  }, [p]);

  const hotspots = useMemo(() => (design && at ? hotspotsOf(design, at) : []), [design, at]);
  // After-delay links fire once the frame has been shown (its transition done) for their delay.
  // The timer calls the latest go, so it follows from the player as it is then.
  const goRef = useRef(go);
  useEffect(() => {
    goRef.current = go;
  }, [go]);
  const settled = !move;
  useEffect(() => {
    if (!settled) return;
    const timers = hotspots.filter((l) => l.trigger === "after-delay").map((l) => setTimeout(() => goRef.current(l), l.delay_ms ?? 0));
    return () => timers.forEach(clearTimeout);
  }, [visit, hotspots, settled]);

  useEffect(() => {
    root.current?.requestFullscreen?.().catch(() => flash("Press F for fullscreen"));
    arm();
    return () => {
      clearTimeout(idle.current);
      clearTimeout(hintTimer.current);
    };
  }, [arm, flash]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      wake();
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const k = e.key;
      if (k === "ArrowLeft" || k === "Backspace") {
        e.preventDefault();
        back();
      } else if (k === "r" || k === "R") again();
      else if (k === "h" || k === "H") setHints((h) => !h);
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
  }, [back, again, help, exit, wake, flash]);

  // The transition: the new frame enters over (or pushes) the old one, which is then dropped.
  const enterRef = useRef<HTMLDivElement>(null);
  const leaveRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    if (!move) return;
    const frames = transitionFrames(move.transition)!;
    const opts: KeyframeAnimationOptions = { duration: move.duration, easing: "ease-out", fill: "both" };
    const a = enterRef.current?.animate(frames.enter, opts);
    // Going back, the old frame is on top: a dissolve fades it out (slides move it away).
    const fadeOut = move.back && !move.transition.startsWith("slide");
    leaveRef.current?.animate(fadeOut ? [{ opacity: 1 }, { opacity: 0 }] : frames.leave, opts);
    let live = true;
    // Without the element (it never mounted) there's nothing to wait for.
    (a?.finished ?? Promise.resolve()).then(
      () => live && setMove(undefined),
      () => undefined,
    );
    return () => {
      live = false;
    };
  }, [move]);

  const frameName = (f?: string) => (f && design ? (screens(design).find((s) => s.id === f)?.name ?? "Frame") : "");
  const count = design ? screens(design).length : 0;
  const laid = at ? laidOf(at) : undefined;
  const leaving = move && move.from !== at ? laidOf(move.from) : undefined;

  return (
    <div
      ref={root}
      className={cx("fixed inset-0 overflow-hidden bg-[#111]", !controls && "cursor-none")}
      onPointerMove={wake}
      onClick={() => {
        // A click that misses every hotspot shows where they are, as prototype players do.
        setHints(true);
        clearTimeout(hintTimer.current);
        hintTimer.current = setTimeout(() => setHints(false), 700);
      }}
    >
      {error ? (
        <p role="alert" className="absolute inset-0 grid place-items-center text-13 text-[#fff9]">{`Couldn’t open this design: ${error}`}</p>
      ) : ref.isError || head.isError ? (
        <p role="alert" className="absolute inset-0 grid place-items-center text-13 text-[#fff9]">Couldn’t open this design.</p>
      ) : !design ? (
        <p role="status" className="absolute inset-0 grid place-items-center"><Spinner size={20} /><span className="sr-only">Loading</span></p>
      ) : !count || !at || !laid ? (
        <p role="status" className="absolute inset-0 grid place-items-center text-13 text-[#fff9]">This design has no frames to present.</p>
      ) : (
        <>
          {leaving && move && (
            <div ref={leaveRef} className="absolute inset-0" aria-hidden style={{ zIndex: move.back ? 1 : 0 }}>
              <FrameView design={design} laid={leaving} frame={move.from} hotspots={[]} hints={false} assetUrl={assetUrl} label="" />
            </div>
          )}
          <div ref={enterRef} key={at} className="absolute inset-0" data-testid="present-frame" data-at={at} style={{ zIndex: move?.back ? 0 : 1 }}>
            <FrameView design={design} laid={laid} frame={at} hotspots={hotspots} hints={hints} onLink={go} assetUrl={assetUrl} label={frameName(at)} />
          </div>
        </>
      )}
      <p aria-live="polite" className="sr-only">{at ? frameName(at) : ""}</p>
      <div
        ref={bar}
        className={cx("absolute bottom-5 left-1/2 z-10 flex -translate-x-1/2 items-center gap-1 rounded-xl border border-border-float bg-surface-raised/95 px-2 py-1.5 shadow-[var(--shadow-float)] transition-opacity", controls || help ? "opacity-100" : "pointer-events-none opacity-0")}
        onFocus={wake}
        onClick={(e) => e.stopPropagation()}
      >
        <button type="button" aria-label="Back" className="grid size-[30px] place-items-center rounded-md text-18 text-fg hover:bg-hover disabled:opacity-40" disabled={!p?.back.length} onClick={back}>‹</button>
        <span className="min-w-[96px] truncate px-1 text-center text-13 text-fg">{frameName(at)}</span>
        <button type="button" aria-label="Restart" className="grid size-[30px] place-items-center rounded-md text-15 text-fg hover:bg-hover" onClick={again}>↺</button>
        <span aria-hidden className="mx-1 h-5 w-px bg-border-strong" />
        <button type="button" aria-label="Keyboard shortcuts" aria-expanded={help} className="grid size-[30px] place-items-center rounded-md text-13 font-semibold text-fg-secondary hover:bg-hover" onClick={() => setHelp((h) => !h)}>?</button>
        <button type="button" onClick={exit} className="ml-1 inline-flex h-[30px] items-center gap-2 rounded-md border border-btn-secondary-border bg-btn-secondary px-3 text-13 font-medium text-fg hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent">
          Exit <span className="font-mono text-11 text-fg-tertiary">esc</span>
        </button>
      </div>
      {help && (
        <div role="dialog" aria-label="Keyboard shortcuts" className="absolute right-6 top-6 z-10 w-[300px] rounded-[14px] border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)]" onClick={(e) => e.stopPropagation()}>
          <p className="mb-2 text-14 font-semibold text-fg">Keyboard</p>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-13">
            {[["← Backspace", "Back"], ["R", "Restart from the first frame"], ["H", "Show or hide hotspots"], ["F", "Toggle fullscreen"], ["Esc", "Exit to the editor"], ["?", "Show or hide this"]].map(([k, d]) => (
              <div key={k} className="contents">
                <dt className="font-mono text-12 text-fg-2">{k}</dt>
                <dd className="text-right text-fg-secondary">{d}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}
      {toast && (
        <div role="status" className="absolute left-1/2 top-5 z-10 -translate-x-1/2 rounded-[10px] border border-border-float bg-surface-raised px-3 py-1.5 text-13 text-fg-2 shadow-[var(--shadow-float)]">
          {toast}
          <button type="button" className="ml-2 text-accent" onClick={() => setToast(undefined)} aria-label="Dismiss">×</button>
        </div>
      )}
    </div>
  );
}
