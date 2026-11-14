import assert from "node:assert/strict";
import test from "node:test";
import { claimPlayback, releasePlayback } from "./audio-playback.ts";

// A stand-in for an <audio> element: all the module touches is paused and pause().
const clip = (playing = false) => ({ paused: !playing, pause() { this.paused = true; } });

test("starting a clip pauses the one that was playing", () => {
  const a = clip(true);
  const b = clip(true);
  claimPlayback(a);
  claimPlayback(b);
  assert.equal(a.paused, true, "the first clip stopped");
  assert.equal(b.paused, false, "the second keeps playing");
});

test("a clip claiming playback again doesn't pause itself", () => {
  const a = clip(true);
  claimPlayback(a);
  claimPlayback(a);
  assert.equal(a.paused, false);
});

test("a released clip is forgotten, so a later one pauses nothing", () => {
  const a = clip(true);
  claimPlayback(a);
  releasePlayback(a);
  const b = clip(true);
  claimPlayback(b);
  assert.equal(a.paused, false, "a released clip is left alone");
  assert.equal(b.paused, false);
});

test("releasing a clip that isn't the current one changes nothing", () => {
  const a = clip(true);
  const b = clip(true);
  claimPlayback(a);
  releasePlayback(b);
  claimPlayback(clip(true));
  assert.equal(a.paused, true, "a was still the current clip, so the next claim paused it");
});
