import assert from "node:assert/strict";
import test from "node:test";
import { clock, plannedMeta, posterFrame, previewFrame, renderEta, renderTook, retryRequest, stageStates } from "./chat-video.ts";

test("stageStates marks earlier stages done and the job's own active or failed", () => {
  assert.deepEqual(stageStates({ stage: "render", state: "running" }), ["done", "done", "active", "pending", "pending"]);
  assert.deepEqual(stageStates({ stage: "encode", state: "failed" }), ["done", "done", "done", "failed", "pending"]);
  assert.deepEqual(stageStates({ stage: "save", state: "done" }), ["done", "done", "done", "done", "done"]);
  assert.deepEqual(stageStates({ stage: "write", state: "running" }), ["active", "pending", "pending", "pending", "pending"]);
});

test("clock and plannedMeta read like the player", () => {
  assert.equal(clock(15000), "0:15");
  assert.equal(clock(62_500), "1:02");
  assert.equal(clock(3_723_000), "1:02:03");
  assert.equal(plannedMeta({ width: 1920, height: 1080, fps: 30, duration_ms: 15000 }), "1920×1080 · 30 fps · 15 s");
  assert.equal(plannedMeta({ width: 640, height: 360, fps: 24, duration_ms: 2500 }), "640×360 · 24 fps · 2.5 s");
});

test("renderEta extrapolates from the frames done so far", () => {
  const start = Date.parse("2026-11-10T00:00:00Z");
  const v = { frames_done: 225, frames_total: 450, render_started_at: "2026-11-10T00:00:00Z" };
  assert.equal(renderEta(v, start + 100_000), "Frame 225 of 450 · ~1m 40s left");
  assert.equal(renderEta({ ...v, frames_done: 0 }, start + 5000), "Frame 0 of 450");
  assert.equal(renderEta({ ...v, frames_total: 0 }, start), "Starting the render…");
  assert.equal(renderEta({ ...v, frames_done: 450 }, start + 5000), "Frame 450 of 450");
});

test("renderTook formats the render's wall time", () => {
  assert.equal(renderTook({ render_started_at: "2026-11-10T00:00:00Z", finished_at: "2026-11-10T00:03:08Z" }), "3m 08s");
  assert.equal(renderTook({ render_started_at: "2026-11-10T00:00:00Z", finished_at: "2026-11-10T00:00:42Z" }), "42s");
  assert.equal(renderTook({ finished_at: "2026-11-10T00:00:42Z" }), null);
});

test("previewFrame and posterFrame pick the best picture available", () => {
  const f = (kind, idx) => ({ kind, idx, version: 1 });
  const v = { frames: [f("still", 0), f("still", 120), f("still", 300), f("strip", 3), f("strip", 11)] };
  assert.deepEqual(previewFrame(v), f("strip", 11));
  assert.deepEqual(posterFrame(v), f("still", 120));
  assert.deepEqual(previewFrame({ frames: [...v.frames, f("preview", 0)] }), f("preview", 0));
  assert.deepEqual(posterFrame({ frames: [...v.frames, f("poster", 0)] }), f("poster", 0));
  assert.equal(previewFrame({ frames: [] }), undefined);
});

test("retryRequest names the step that failed", () => {
  assert.equal(retryRequest({ title: "Black cat", stage: "encode" }), 'Retry the video "Black cat" from the encode step.');
});
