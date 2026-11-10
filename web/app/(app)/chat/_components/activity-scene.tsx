"use client";

import { useEffect, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from "react";
import type { ChatStatusKind } from "@/lib/chat-ui";

/** How long a scene stays up at least, so rapid tool calls never strobe. */
const HOLD_MS = 800;
/** Matches the crossfade in globals.css. */
const FADE_MS = 240;
/** How long a scene takes to coast to a stop once the person presses Stop. */
const WIND_DOWN_MS = 600;

const delay = (seconds: number): CSSProperties => ({ animationDelay: `${seconds}s` });

/**
 * The scene to show for the newest kind of work: it follows `kind`, but holds each scene
 * for HOLD_MS and keeps the outgoing one mounted for the crossfade. Kinds that come and
 * go inside a hold are skipped.
 */
export function useHeldScene(kind: ChatStatusKind) {
  const [scene, setScene] = useState<{ current: ChatStatusKind; previous?: ChatStatusKind }>({ current: kind });
  const current = scene.current;
  const shownAt = useRef(0);
  useEffect(() => {
    shownAt.current = Date.now();
  }, []);
  useEffect(() => {
    if (kind === current) return;
    const timer = setTimeout(() => {
      shownAt.current = Date.now();
      setScene((s) => ({ current: kind, previous: s.current }));
    }, Math.max(0, HOLD_MS - (Date.now() - shownAt.current)));
    return () => clearTimeout(timer);
  }, [kind, current]);
  useEffect(() => {
    if (!scene.previous) return;
    const timer = setTimeout(() => setScene((s) => ({ current: s.current })), FADE_MS);
    return () => clearTimeout(timer);
  }, [scene.previous]);
  return scene;
}

const LABELS: Record<ChatStatusKind, string> = {
  thinking: "Thinking",
  connector: "Using a connector",
  secret: "Using a secret",
  wiki: "Reading the wiki",
  tickets: "Working on tickets",
  "reading-code": "Reading code",
  "searching-code": "Searching code",
  "writing-code": "Writing code",
  tests: "Running tests",
  build: "Checking the build",
  install: "Installing dependencies",
  terminal: "Working in the terminal",
  github: "Working on GitHub",
  commit: "Committing changes",
  push: "Pushing to the repository",
  "pull-request": "Preparing a pull request",
  "repo-history": "Looking over the repository",
  web: "Searching the web",
  "web-page": "Reading a page",
  video: "Watching a video",
  image: "Creating an image",
  "image-view": "Looking at an image",
  canvas: "Working on a canvas",
  "canvas-comment": "Commenting on a canvas",
  runs: "Checking on runs",
  workflow: "Working on a workflow",
  memory: "Memory",
  "memory-save": "Making a note",
  "chat-history": "Searching past chats",
};

export const sceneLabel = (kind: ChatStatusKind) => LABELS[kind];

const CODE_LINES = "M20 10H72 M30 19H112 M30 28H88 M20 37H60 M20 46H108";

const toX = (px: number) => ({ "--to-x": `${px}px` }) as CSSProperties;

function canvasScene(pin?: boolean) {
  return (
    <>
      <rect className="scene-paper" x="30" y="4" width="72" height="48" rx="3" />
      {["M42 14H70V32H42Z", "M84 22a8 8 0 1 0 .01 0", "M42 42H90"].map((d, i) => (
        <path key={d} className="scene-line-accent scene-link" d={d} pathLength={1} style={delay(i * 0.35)} />
      ))}
      {pin && (
        <g className="scene-box scene-pin">
          <path className="scene-warm" d="M92 20c-4-5-6-7-6-10a6 6 0 0 1 12 0c0 3-2 5-6 10z" />
        </g>
      )}
      <g className="scene-cursor">
        <path fill="var(--text)" d="M70 32v9l2.6-2.4 3.6-.2z" />
      </g>
    </>
  );
}

/** Kinds split off a broader one keep its scene: the step rail's glyph is what tells them apart. */
type SplitKind = "searching-code" | "web-page" | "image-view" | "memory-save";

const SCENES: Record<Exclude<ChatStatusKind, SplitKind>, ReactNode> = {
  thinking: (
    <>
      <circle className="scene-box scene-breathe" cx="66" cy="28" r="22" fill="none" stroke="var(--accent)" strokeOpacity="0.35" strokeWidth="1.5" />
      <circle className="scene-accent scene-drift-a" cx="54" cy="29" r="4" opacity="0.55" />
      <circle className="scene-accent scene-drift-b" cx="66" cy="23" r="5" />
      <circle className="scene-accent scene-drift-c" cx="78" cy="31" r="3.5" opacity="0.75" />
    </>
  ),
  wiki: (
    <>
      <rect className="scene-paper" x="38" y="5" width="56" height="46" rx="4" />
      <rect className="scene-accent scene-sweep" x="38" y="18" width="56" height="9" opacity="0.3" />
      <path className="scene-line" d="M45 13H85 M45 22H81 M45 31H87 M45 40H70" />
    </>
  ),
  tickets: (
    <>
      {[24, 58, 92].map((x) => (
        <rect key={x} className="scene-empty" x={x} y="5" width="26" height="46" rx="3" />
      ))}
      <rect className="scene-paper" x="27" y="36" width="20" height="10" rx="2" />
      <rect className="scene-paper" x="61" y="36" width="20" height="10" rx="2" />
      <rect className="scene-paper" x="95" y="26" width="20" height="10" rx="2" />
      <rect className="scene-accent scene-card" x="27" y="12" width="20" height="10" rx="2" />
    </>
  ),
  "reading-code": (
    <>
      <path className="scene-line" d={CODE_LINES} />
      <rect className="scene-accent scene-scan" x="14" y="23" width="104" height="10" rx="2" opacity="0.25" />
    </>
  ),
  "writing-code": (
    <>
      <path className="scene-line-accent scene-left scene-type-1" d="M20 12H80" />
      <path className="scene-line scene-left scene-type-2" d="M30 22H110" />
      <path className="scene-line-done scene-left scene-type-3" d="M30 32H80" />
      <path className="scene-line-done scene-left scene-type-4" d="M30 42H62" />
      <g className="scene-caret-path">
        <rect className="scene-caret" x="63" y="38" width="1.5" height="8" fill="var(--text)" />
      </g>
    </>
  ),
  tests: (
    <>
      {[26, 46, 66, 86, 106].map((cx, i) => (
        <g key={cx}>
          <circle className="scene-empty" cx={cx} cy="28" r="6.5" />
          <circle className="scene-done scene-box scene-dot" cx={cx} cy="28" r="6.5" style={{ ...delay(i * 0.4), ...(i === 4 ? { transform: "scale(0)" } : {}) }} />
        </g>
      ))}
    </>
  ),
  build: (
    <>
      <path className="scene-line" d="M36 53H96" />
      <g className="scene-row-1">
        {[42, 58, 74].map((x) => (
          <rect key={x} className="scene-paper" x={x} y="40" width="15" height="12" rx="2" />
        ))}
      </g>
      <g className="scene-row-2">
        {[50, 66].map((x) => (
          <rect key={x} className="scene-paper" x={x} y="27" width="15" height="12" rx="2" />
        ))}
      </g>
      <g className="scene-row-3">
        <rect className="scene-accent" x="58" y="14" width="15" height="12" rx="2" />
      </g>
    </>
  ),
  install: (
    <>
      {[52, 62, 72].map((x, i) => (
        <rect key={x} className="scene-accent scene-drop" x={x} y="6" width="9" height="9" rx="1.5" style={delay(i * 0.8)} />
      ))}
      <g className="scene-bump">
        <rect className="scene-paper" x="46" y="30" width="40" height="22" rx="2" />
        <path className="scene-line" d="M46 30L38 24 M86 30L94 24 M60 41H72" />
      </g>
    </>
  ),
  terminal: (
    <>
      <rect className="scene-paper" x="28" y="4" width="76" height="48" rx="4" />
      <svg x="36" y="8" width="60" height="30" viewBox="0 0 60 30">
        <path className="scene-line scene-scroll-3" d="M0 5H42 M0 15H54 M0 25H30 M0 35H42 M0 45H54 M0 55H30" />
      </svg>
      <path className="scene-line-accent" d="M36 40l3 3-3 3" />
      <rect className="scene-caret" x="44" y="39" width="5" height="8" fill="var(--accent)" />
    </>
  ),
  github: (
    <>
      <path className="scene-line" d="M8 44H124" />
      <path className="scene-line-done" d="M24 44C40 44 40 14 56 14H88C104 14 104 44 120 44" />
      <circle className="scene-muted" cx="24" cy="44" r="3.5" />
      <circle className="scene-done" cx="56" cy="14" r="3.5" />
      <circle cx="120" cy="44" r="4" fill="var(--surface)" stroke="var(--accent)" strokeWidth="1.5" />
      <g className="scene-commit">
        <circle className="scene-done" cx="72" cy="14" r="7" opacity="0.25" />
        <circle className="scene-done" cx="72" cy="14" r="3" />
      </g>
    </>
  ),
  commit: (
    <>
      <path className="scene-line" d="M12 40H120" />
      <circle className="scene-muted" cx="28" cy="40" r="3.5" />
      <circle className="scene-muted" cx="64" cy="40" r="3.5" />
      {[12, 0, -12].map((x, i) => (
        <rect key={x} className="scene-paper scene-box scene-chip" x={96 - x} y="8" width="8" height="10" rx="1.5" style={{ ...toX(x), ...delay(i * 0.15) }} />
      ))}
      <circle className="scene-done scene-box scene-stamp" cx="100" cy="40" r="5" />
    </>
  ),
  push: (
    <>
      <path className="scene-line" d="M20 44L88 22" />
      <circle className="scene-muted" cx="20" cy="44" r="4" />
      <path className="scene-line-accent scene-box scene-arrive" d="M92 28a8 8 0 0 1 0-16 11 11 0 0 1 21-2 9 9 0 0 1 3 18z" />
      <g className="scene-push">
        <circle className="scene-done" cx="88" cy="22" r="7" opacity="0.25" />
        <circle className="scene-done" cx="88" cy="22" r="3" />
      </g>
    </>
  ),
  "pull-request": (
    <>
      <path className="scene-line" d="M10 44H122" />
      <path className="scene-line-done scene-link" d="M22 44C36 44 36 14 52 14H70C86 14 86 44 100 44" pathLength={1} />
      <circle className="scene-muted" cx="22" cy="44" r="3.5" />
      <g className="scene-box scene-pop">
        <circle className="scene-done" cx="100" cy="44" r="8" />
        <path d="M96.5 44l2.5 2.5 4.5-5" fill="none" stroke="var(--surface)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      </g>
    </>
  ),
  "repo-history": (
    <>
      <path className="scene-line" d="M30 6V50" />
      {[10, 22, 34, 46].map((cy) => (
        <circle key={cy} className="scene-muted" cx="30" cy={cy} r="3" />
      ))}
      <path className="scene-line" d="M42 10H104 M42 22H88 M42 34H110 M42 46H78" />
      <rect className="scene-accent scene-log" x="22" y="4" width="94" height="12" rx="2" opacity="0.25" />
    </>
  ),
  web: (
    <>
      <g fill="none" stroke="var(--text-tertiary)" strokeWidth="1.5">
        <circle cx="66" cy="28" r="16" />
        <ellipse cx="66" cy="28" rx="7" ry="16" />
        <path d="M50 28H82 M53 19H79 M53 37H79" />
      </g>
      <g className="scene-orbit">
        <g className="scene-upright">
          <circle cx="79" cy="19" r="6.5" fill="var(--surface)" stroke="var(--accent)" strokeWidth="2" />
          <path className="scene-line-accent" d="M83.6 23.6l5.4 5.4" />
        </g>
      </g>
    </>
  ),
  video: (
    <>
      <rect className="scene-paper" x="40" y="3" width="52" height="31" rx="4" />
      <path className="scene-accent scene-box scene-node" d="M61 11v15l12-7.5z" />
      <svg x="34" y="38" width="64" height="16" viewBox="0 0 64 16">
        <path className="scene-line scene-scroll-2" d="M6 4H58 M14 12H50 M6 20H58 M14 28H50" />
      </svg>
    </>
  ),
  image: (
    <>
      {["var(--border-strong)", "var(--text-tertiary)", "var(--accent)", "var(--accent)", "var(--text-tertiary)", "var(--accent)", "var(--accent)", "var(--state-progress)", "var(--border-strong)", "var(--state-done)", "var(--state-done)", "var(--state-done)"].map((fill, i) => (
        <rect key={i} className="scene-tile" x={40 + (i % 4) * 13} y={6 + Math.floor(i / 4) * 14.67} width="13" height="14.67" fill={fill} opacity={i % 4 === 2 ? 0.75 : 1} style={delay(((i * 5) % 12) * 0.08)} />
      ))}
    </>
  ),
  canvas: canvasScene(),
  "canvas-comment": canvasScene(true),
  runs: (
    <>
      {[
        { y: 14, rest: 0.8, seconds: 2.6, stroke: "var(--accent)" },
        { y: 28, rest: 0.45, seconds: 3.7, stroke: "var(--state-done)" },
        { y: 42, rest: 0.65, seconds: 3.1, stroke: "var(--state-progress)" },
      ].map(({ y, rest, seconds, stroke }) => (
        <g key={y}>
          <path className="scene-line" d={`M24 ${y}H108`} opacity="0.3" />
          <path className="scene-line scene-left scene-bar" d={`M24 ${y}H108`} style={{ stroke, transform: `scaleX(${rest})`, animationDuration: `${seconds}s` }} />
        </g>
      ))}
    </>
  ),
  workflow: (
    <>
      <path className="scene-line" d="M30 28C42 28 44 14 56 14 M30 28C42 28 44 42 56 42 M74 14C86 14 88 28 98 28 M74 42C86 42 88 28 98 28" />
      {[
        { x: 12, y: 22, wait: 0 },
        { x: 56, y: 8, wait: 0.5 },
        { x: 56, y: 36, wait: 0.5 },
        { x: 98, y: 22, wait: 1 },
      ].map(({ x, y, wait }) => (
        <rect key={`${x}-${y}`} className="scene-accent scene-light" x={x} y={y} width="18" height="12" rx="3" style={delay(wait)} />
      ))}
    </>
  ),
  connector: (
    <>
      <rect className="scene-accent scene-box scene-plug" x="24" y="19" width="26" height="18" rx="4" />
      <path className="scene-line-accent scene-plug" d="M8 28H24M50 23H60M50 33H60" />
      <rect className="scene-warm" x="82" y="17" width="26" height="22" rx="4" fillOpacity="0.85" />
      <path className="scene-line-accent" d="M108 28H124" />
      <path className="scene-line-accent scene-link" d="M64 23H82M64 33H82" pathLength={1} strokeOpacity="0.5" />
      <circle className="scene-warm scene-box scene-node" cx="73" cy="10" r="2.5" />
      <circle className="scene-accent scene-box scene-node" cx="66" cy="47" r="2" style={delay(0.5)} />
      <circle className="scene-warm scene-box scene-node" cx="80" cy="47" r="2" style={delay(1)} />
    </>
  ),
  // A key travels to a lock: Vesper adds the secret on the way out.
  secret: (
    <>
      <circle className="scene-warm scene-box" cx="30" cy="28" r="9" fillOpacity="0.85" />
      <path className="scene-line-accent scene-plug" d="M39 28H62M54 28V34M60 28V33" />
      <rect className="scene-accent scene-box" x="86" y="22" width="22" height="18" rx="3" />
      <path className="scene-line-accent" d="M90 22V17A7 7 0 0 1 104 17V22" />
      <path className="scene-line-accent scene-link" d="M64 28H84" pathLength={1} strokeOpacity="0.5" />
      <circle className="scene-warm scene-box scene-node" cx="97" cy="31" r="2" style={delay(0.5)} />
    </>
  ),
  memory: (
    <>
      {["M30 44L54 16", "M54 16L84 32", "M84 32L106 10", "M54 16L72 50", "M72 50L84 32"].map((d, i) => (
        <path key={d} className="scene-line-accent scene-link" d={d} pathLength={1} strokeOpacity="0.6" style={delay(i * 0.35)} />
      ))}
      <circle className="scene-accent scene-box scene-node" cx="30" cy="44" r="3.5" />
      <circle className="scene-accent scene-box scene-node" cx="54" cy="16" r="4.5" style={delay(0.3)} />
      <circle className="scene-accent scene-box scene-node" cx="84" cy="32" r="4" style={delay(0.6)} />
      <circle className="scene-warm scene-box scene-node" cx="106" cy="10" r="5" style={delay(0.9)} />
      <circle className="scene-accent scene-box scene-node" cx="72" cy="50" r="3" style={delay(1.2)} />
    </>
  ),
  // Past conversations as speech bubbles; a lens passes over each in turn and
  // the one under it lights up, like a match turning up.
  "chat-history": (
    <>
      {[
        { x: 22, y: 4, w: 48, tail: "M30 16l-4 5 9-5" },
        { x: 62, y: 21, w: 48, tail: "M102 33l4 5-9-5" },
        { x: 22, y: 38, w: 40, tail: "" },
      ].map(({ x, y, w, tail }, i) => (
        <g key={y}>
          {tail && <path className="scene-paper" d={tail} />}
          <rect className="scene-paper" x={x} y={y} width={w} height="13" rx="6.5" />
          <rect className="scene-accent scene-history-hit" x={x} y={y} width={w} height="13" rx="6.5" style={delay(i * 1.32)} />
          <path className="scene-line" d={`M${x + 7} ${y + 6.5}H${x + w - 12}`} strokeWidth="1.5" />
        </g>
      ))}
      <g className="scene-history-lens">
        <circle cx="46" cy="10.5" r="7" fill="var(--surface)" fillOpacity="0.4" stroke="var(--accent)" strokeWidth="2" />
        <path className="scene-line-accent" d="M51 15.5l5 5" />
      </g>
    </>
  ),
};

const SCENE_OF: Record<SplitKind, Exclude<ChatStatusKind, SplitKind>> = {
  "searching-code": "reading-code",
  "web-page": "web",
  "image-view": "image",
  "memory-save": "memory",
};

const sceneOf = (kind: ChatStatusKind) => (kind in SCENE_OF ? SCENE_OF[kind as SplitKind] : (kind as Exclude<ChatStatusKind, SplitKind>));

function SceneLayer({ kind, leaving }: { kind: ChatStatusKind; leaving?: boolean }) {
  return (
    <div data-scene={leaving ? undefined : kind} className={`scene-layer ${leaving ? "scene-out" : "scene-in"}`}>
      <svg viewBox="0 0 132 56" className={`h-full w-full ${sceneOf(kind) === "image" ? "scene-focus" : ""}`}>
        {SCENES[sceneOf(kind)]}
      </svg>
    </div>
  );
}

/** Coasts the looping animations under `ref` to a halt while `stopping`, and restores them if the stop doesn't take. */
function useWindDown(ref: RefObject<HTMLElement | null>, stopping: boolean | undefined, scene: ChatStatusKind) {
  useEffect(() => {
    if (!stopping || !ref.current) return;
    const loops = ref.current.getAnimations({ subtree: true }).filter((a) => a.effect?.getTiming().iterations === Infinity);
    const start = performance.now();
    let frame = requestAnimationFrame(function step(now) {
      const rate = Math.max(0, 1 - (now - start) / WIND_DOWN_MS);
      for (const a of loops) a.playbackRate = rate;
      if (rate > 0) frame = requestAnimationFrame(step);
    });
    return () => {
      cancelAnimationFrame(frame);
      for (const a of loops) a.playbackRate = 1;
    };
  }, [ref, stopping, scene]);
}

/** The card's vignette, purely decorative: the status text is what's announced. Hidden when the card is too narrow. */
export function ActivityScene({ current, previous, stopping }: { current: ChatStatusKind; previous?: ChatStatusKind; stopping?: boolean }) {
  const stage = useRef<HTMLDivElement>(null);
  useWindDown(stage, stopping, current);
  return (
    <div
      ref={stage}
      aria-hidden
      data-stopping={stopping || undefined}
      className={`relative h-14 w-[132px] flex-none transition-[filter,opacity] duration-500 motion-reduce:transition-none @max-[420px]:w-[88px] @max-[300px]:hidden ${stopping ? "opacity-50 grayscale" : ""}`}
    >
      {previous && <SceneLayer key={`out-${previous}`} kind={previous} leaving />}
      <SceneLayer key={current} kind={current} />
    </div>
  );
}
