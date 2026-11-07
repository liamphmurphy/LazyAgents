import assert from "node:assert/strict";
import test from "node:test";
import { tokenBreakdown, eventTokenUsage } from "./token-usage.ts";

test("cache reads are included in input and not added twice", () => {
  const usage = tokenBreakdown({ input_tokens: 1000, output_tokens: 100, cached_input_tokens: 980, uncached_input_tokens: 20 });
  assert.equal(usage.total, 1100);
  assert.equal(usage.cached, 980);
  assert.equal(usage.unknown, 0);
});
test("absent cache coverage remains unknown rather than uncached", () => {
  const usage = tokenBreakdown({ input_tokens: 1000, output_tokens: 10 });
  assert.equal(usage.unknown, 1000);
  assert.equal(usage.uncached, 0);
  assert.equal(usage.hasBreakdown, false);
});
test("mixed provider coverage is retained and invalid counters are clamped", () => {
  assert.equal(tokenBreakdown({ input_tokens: 100, cached_input_tokens: 30, uncached_input_tokens: 20 }).unknown, 50);
  assert.equal(tokenBreakdown({ input_tokens: 100, cached_input_tokens: 1000, uncached_input_tokens: 40 }).cached, 100);
});
test("usage events distinguish explicit zero hits from missing counters", () => {
  assert.equal(tokenBreakdown(eventTokenUsage({ input_tokens: 100, cached_tokens: 0, cache_reported_input_tokens: 100 })).uncached, 100);
  assert.equal(tokenBreakdown(eventTokenUsage({ input_tokens: 100, cached_tokens: 0, cache_reported_input_tokens: 0 })).unknown, 100);
  assert.equal(tokenBreakdown(eventTokenUsage({ input_tokens: 100, cached_tokens: 90 })).cached, 90);
});
