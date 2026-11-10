"use client";

import { useRef, type KeyboardEvent, type PointerEvent } from "react";
import { wheelPoint, wheelPosition } from "@/lib/persona-ui";

const WHEEL = 128;

/** Hue around, saturation outwards, at 50% lightness (the slider sets the rest). */
export function ColorWheel({ h, s, onPick }: { h: number; s: number; onPick: (h: number, s: number) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const r = WHEEL / 2;
  const pick = (e: PointerEvent) => {
    const box = ref.current!.getBoundingClientRect();
    const p = wheelPoint(e.clientX - box.left - r, e.clientY - box.top - r, r);
    onPick(p.h, p.s);
  };
  const key = (e: KeyboardEvent) => {
    const step = { ArrowLeft: [-5, 0], ArrowRight: [5, 0], ArrowUp: [0, 0.05], ArrowDown: [0, -0.05] }[e.key];
    if (!step) return;
    e.preventDefault();
    onPick((h + step[0] + 360) % 360, Math.min(1, Math.max(0, s + step[1])));
  };
  const dot = wheelPosition(h, s, r);
  return (
    <div
      ref={ref}
      role="slider"
      tabIndex={0}
      aria-label="Color wheel: arrows change hue and saturation"
      aria-valuemin={0}
      aria-valuemax={359}
      aria-valuenow={Math.round(h)}
      aria-valuetext={`hue ${Math.round(h)}°, saturation ${Math.round(s * 100)}%`}
      onPointerDown={(e) => {
        e.currentTarget.setPointerCapture(e.pointerId);
        pick(e);
      }}
      onPointerMove={(e) => e.buttons === 1 && pick(e)}
      onKeyDown={key}
      className="relative flex-none cursor-crosshair touch-none rounded-full outline-none focus-visible:ring-2 focus-visible:ring-accent"
      style={{
        width: WHEEL,
        height: WHEEL,
        background:
          "radial-gradient(circle closest-side, #808080, transparent), conic-gradient(from 90deg, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00)",
      }}
    >
      <span
        aria-hidden
        className="pointer-events-none absolute size-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-current shadow"
        style={{ left: r + dot.x, top: r + dot.y }}
      />
    </div>
  );
}

