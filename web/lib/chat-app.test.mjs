import test from "node:test";
import assert from "node:assert/strict";
import { APP_CSP, APP_MAX_HEIGHT, APP_MIN_HEIGHT, appDocument, appHeight, isAppFence } from "./chat-app.ts";

test("app fences are recognised by their first info word", () => {
  assert.equal(isAppFence("app"), true);
  assert.equal(isAppFence(" APP title=Counter"), true);
  assert.equal(isAppFence("html-app"), true);
  assert.equal(isAppFence("html"), false);
  assert.equal(isAppFence("ts"), false);
  assert.equal(isAppFence(undefined), false);
});

test("the CSP comes before the app's markup and blocks the network", () => {
  const doc = appDocument("<!DOCTYPE html>\n<html><head><script>fetch('/x')</script></head><body>hi</body></html>");
  assert.ok(doc.startsWith("<!doctype html><meta charset=\"utf-8\"><meta http-equiv=\"Content-Security-Policy\""));
  assert.ok(doc.indexOf("Content-Security-Policy") < doc.indexOf("fetch('/x')"));
  // The app's own doctype is dropped, so only ours remains.
  assert.equal(doc.match(/<!doctype/gi).length, 1);
  assert.match(APP_CSP, /connect-src 'none'/);
  assert.match(APP_CSP, /default-src 'none'/);
  assert.match(APP_CSP, /form-action 'none'/);
  assert.doesNotMatch(APP_CSP, /img-src[^;]*https/);
});

test("a bare fragment becomes a full document", () => {
  const doc = appDocument("<button id=b>Click</button><script>b.onclick=()=>{}</script>");
  assert.ok(doc.endsWith("<button id=b>Click</button><script>b.onclick=()=>{}</script>"));
  assert.match(doc, /vesper-app:resize/);
});

test("reported heights are clamped", () => {
  assert.equal(appHeight(10), APP_MIN_HEIGHT);
  assert.equal(appHeight(300.2), 301);
  assert.equal(appHeight(99999), APP_MAX_HEIGHT);
  assert.equal(appHeight("300"), APP_MIN_HEIGHT);
  assert.equal(appHeight(Number.NaN), APP_MIN_HEIGHT);
});
