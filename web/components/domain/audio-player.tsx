"use client";

import { useEffect, useRef, useState } from "react";
import { clock } from "@/lib/chat-video";
import { nextSpeed, skip, speedLabel } from "@/lib/chat-audio";
import { claimPlayback, releasePlayback } from "@/lib/audio-playback";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { Waveform } from "./waveform";

/**
 * Plays one audio file: the waveform scrubs, with play and pause, ±10 s, speed and loop. Only one
 * clip plays at a time, and playback stops when the player goes away (a closed dialog, a new page).
 */
export function AudioPlayer({
  src, peaks, durationMs, large, autoPlay, className,
}: { src: string; peaks?: readonly number[]; durationMs?: number; large?: boolean; autoPlay?: boolean; className?: string }) {
  const ref = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [at, setAt] = useState(0);
  const [length, setLength] = useState((durationMs ?? 0) / 1000);
  const [rate, setRate] = useState(1);
  const [loop, setLoop] = useState(false);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    const el = ref.current;
    return () => {
      if (!el) return;
      el.pause();
      releasePlayback(el);
    };
  }, []);
  const toggle = () => {
    const el = ref.current;
    if (!el) return;
    if (el.paused) void el.play().catch(() => setFailed(true));
    else el.pause();
  };
  const seekTo = (s: number) => {
    const el = ref.current;
    if (el) el.currentTime = s;
    setAt(s);
  };
  const btn = "grid place-items-center rounded-full text-fg hover:text-accent disabled:opacity-40";
  return (
    <div className={cx("flex flex-col gap-2", className)} data-testid="audio-player">
      <audio
        ref={ref}
        src={src}
        preload="metadata"
        loop={loop}
        autoPlay={autoPlay}
        onPlay={(e) => {
          claimPlayback(e.currentTarget);
          setPlaying(true);
        }}
        onPause={() => setPlaying(false)}
        onEnded={() => setPlaying(false)}
        onError={() => setFailed(true)}
        onTimeUpdate={(e) => setAt(e.currentTarget.currentTime)}
        onLoadedMetadata={(e) => Number.isFinite(e.currentTarget.duration) && setLength(e.currentTarget.duration)}
      />
      <Waveform
        peaks={peaks}
        progress={length ? at / length : 0}
        bars={large ? 64 : 40}
        onSeek={(f) => length && seekTo(f * length)}
        className={large ? "h-28 rounded-lg bg-surface-sunken px-3 py-2" : "h-10"}
      />
      <div className="flex items-center gap-2.5 text-xs text-fg-secondary">
        <button type="button" onClick={toggle} aria-label={playing ? "Pause" : "Play"} className={cx(btn, "size-9 bg-accent text-bg hover:text-bg")} data-testid="audio-toggle">
          <Icon name={playing ? "pause" : "play"} size={14} />
        </button>
        <button type="button" aria-label="Back 10 seconds" className={cx(btn, "h-7 px-1.5 text-[11px]")} onClick={() => seekTo(skip(at, -10, length))}>−10s</button>
        <button type="button" aria-label="Forward 10 seconds" className={cx(btn, "h-7 px-1.5 text-[11px]")} onClick={() => seekTo(skip(at, 10, length))}>+10s</button>
        <span className="font-mono tabular-nums" data-testid="audio-time">{clock(at * 1000)} / {length ? clock(length * 1000) : "–:––"}</span>
        <span className="flex-1" />
        <button
          type="button"
          aria-label={`Speed ${speedLabel(rate)}`}
          className={cx(btn, "h-7 px-1.5 font-mono text-[11px]")}
          onClick={() => {
            const next = nextSpeed(rate);
            setRate(next);
            if (ref.current) ref.current.playbackRate = next;
          }}
        >
          {speedLabel(rate)}
        </button>
        <button type="button" aria-label="Loop" aria-pressed={loop} className={cx(btn, "h-7 px-1.5 text-[11px]", loop && "text-accent")} onClick={() => setLoop((x) => !x)}>
          Loop {loop ? "on" : "off"}
        </button>
      </div>
      {failed && <p className="text-xs text-pill-blocked-fg" role="alert">This browser couldn&apos;t play the file. Download it to listen.</p>}
    </div>
  );
}
