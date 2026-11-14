"use client";

import { useRef } from "react";
import { cx } from "@/lib/cx";
import { WAVE_BARS, waveBars } from "@/lib/chat-audio";

/**
 * A clip's waveform: one bar per bucket the server read from the file, the played part in the
 * accent colour. A format the server can't decode draws flat. With onSeek it's a slider: a click
 * or the arrow keys move the position (a fraction, 0–1).
 */
export function Waveform({
  peaks, progress = 0, bars = WAVE_BARS, className, onSeek, label = "Seek",
}: {
  peaks?: readonly number[];
  progress?: number;
  bars?: number;
  className?: string;
  onSeek?: (fraction: number) => void;
  label?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const heights = waveBars(peaks, bars);
  const at = Math.min(1, Math.max(0, progress));
  const seekable = !!onSeek;
  return (
    <div
      ref={ref}
      className={cx("flex items-center gap-[2px]", seekable && "cursor-pointer", className)}
      data-testid="waveform"
      {...(seekable
        ? {
            role: "slider",
            tabIndex: 0,
            "aria-label": label,
            "aria-valuemin": 0,
            "aria-valuemax": 100,
            "aria-valuenow": Math.round(at * 100),
            onClick: (e: React.MouseEvent) => {
              const box = ref.current?.getBoundingClientRect();
              if (box && box.width > 0) onSeek((e.clientX - box.left) / box.width);
            },
            onKeyDown: (e: React.KeyboardEvent) => {
              if (e.key === "ArrowRight") onSeek(Math.min(1, at + 0.05));
              else if (e.key === "ArrowLeft") onSeek(Math.max(0, at - 0.05));
              else return;
              e.preventDefault();
            },
          }
        : { "aria-hidden": true })}
    >
      {heights.map((h, i) => (
        <span
          key={i}
          className={cx("min-w-px flex-1 rounded-full", (i + 0.5) / heights.length <= at ? "bg-accent" : "bg-accent/30")}
          style={{ height: `${Math.round(h * 100)}%` }}
        />
      ))}
    </div>
  );
}
