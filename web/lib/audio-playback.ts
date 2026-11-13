// Only one clip plays at a time: starting one pauses whichever was playing.

let current: HTMLMediaElement | null = null;

/** Call as a clip starts playing. */
export function claimPlayback(el: HTMLMediaElement): void {
  if (current && current !== el && !current.paused) current.pause();
  current = el;
}

/** Call when a clip is removed, so a stale element isn't held. */
export function releasePlayback(el: HTMLMediaElement): void {
  if (current === el) current = null;
}
