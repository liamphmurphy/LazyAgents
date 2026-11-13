import test from "node:test";
import assert from "node:assert/strict";
import { AUDIO_STAGES, audioRetryRequest, audioStageStates, nextSpeed, skip, waveBars } from "./chat-audio.ts";

test("the stages read done before the job's stage, active at it, pending after", () => {
  assert.deepEqual(audioStageStates({ stage: "render", state: "running" }), ["done", "active", "pending", "pending"]);
  assert.deepEqual(audioStageStates({ stage: "save", state: "running" }), ["done", "done", "done", "active"]);
  assert.deepEqual(audioStageStates({ stage: "check", state: "failed" }), ["done", "done", "failed", "pending"]);
  assert.deepEqual(audioStageStates({ stage: "save", state: "done" }), ["done", "done", "done", "done"]);
  assert.equal(AUDIO_STAGES.length, 4);
});

test("waveBars draws the server's peaks, resampled, and a flat line without them", () => {
  assert.deepEqual(waveBars(undefined, 4), [0.08, 0.08, 0.08, 0.08]);
  assert.deepEqual(waveBars([], 2), [0.08, 0.08]);
  assert.deepEqual(waveBars([0, 255, 0, 255], 4), [0.08, 1, 0.08, 1]);
  // Fewer bars than peaks: each bar takes the loudest peak it covers.
  assert.deepEqual(waveBars([10, 255, 0, 51], 2), [1, 51 / 255]);
  // More bars than peaks: each peak repeats.
  assert.deepEqual(waveBars([255, 0], 4), [1, 1, 0.08, 0.08]);
  assert.equal(waveBars(new Array(64).fill(128)).length, 64);
});

test("speed cycles and wraps; skipping stays inside the clip", () => {
  assert.equal(nextSpeed(1), 1.25);
  assert.equal(nextSpeed(2), 0.75);
  assert.equal(nextSpeed(3), 0.75, "an unknown rate starts over");
  assert.equal(skip(5, 10, 12), 12);
  assert.equal(skip(5, -10, 12), 0);
  assert.equal(skip(5, 3, 12), 8);
  assert.equal(skip(5, 3, 0), 0, "an unknown length keeps it at the start");
});

test("Retry names the audio and the step it stopped at", () => {
  assert.equal(audioRetryRequest({ title: "Riff", stage: "render" }), 'Retry the audio "Riff" from the render step.');
  assert.equal(audioRetryRequest({ title: "Riff", stage: "check" }), 'Retry the audio "Riff" from the mix check step.');
});
