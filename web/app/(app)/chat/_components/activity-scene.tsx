"use client";

import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import type { ChatStatusKind } from "@/lib/chat-ui";

/** How long a scene stays up at least, so rapid tool calls never strobe. */
const HOLD_MS = 800;
/** Matches the crossfade in globals.css. */
const FADE_MS = 240;

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
  wiki: "Reading the wiki",
  "reading-code": "Reading code",
  "writing-code": "Writing code",
  tests: "Running tests",
  github: "Working on GitHub",
  web: "Searching the web",
  image: "Creating an image",
  memory: "Memory",
};

export const sceneLabel = (kind: ChatStatusKind) => LABELS[kind];

const CODE_LINES = "M20 10H72 M30 19H112 M30 28H88 M20 37H60 M20 46H108";

const SCENES: Record<ChatStatusKind, ReactNode> = {
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
  image: (
    <>
      {["var(--border-strong)", "var(--text-tertiary)", "var(--accent)", "var(--accent)", "var(--text-tertiary)", "var(--accent)", "var(--accent)", "var(--state-progress)", "var(--border-strong)", "var(--state-done)", "var(--state-done)", "var(--state-done)"].map((fill, i) => (
        <rect key={i} className="scene-tile" x={40 + (i % 4) * 13} y={6 + Math.floor(i / 4) * 14.67} width="13" height="14.67" fill={fill} opacity={i % 4 === 2 ? 0.75 : 1} style={delay(((i * 5) % 12) * 0.08)} />
      ))}
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
};

function SceneLayer({ kind, leaving }: { kind: ChatStatusKind; leaving?: boolean }) {
  return (
    <div data-scene={leaving ? undefined : kind} className={`scene-layer ${leaving ? "scene-out" : "scene-in"}`}>
      <svg viewBox="0 0 132 56" className={`h-full w-full ${kind === "image" ? "scene-focus" : ""}`}>
        {SCENES[kind]}
      </svg>
    </div>
  );
}

/** The card's vignette, purely decorative: the status text is what's announced. Hidden when the card is too narrow. */
export function ActivityScene({ current, previous }: { current: ChatStatusKind; previous?: ChatStatusKind }) {
  return (
    <div aria-hidden className="relative h-14 w-[132px] flex-none @max-[420px]:w-[88px] @max-[300px]:hidden">
      {previous && <SceneLayer key={`out-${previous}`} kind={previous} leaving />}
      <SceneLayer key={current} kind={current} />
    </div>
  );
}
