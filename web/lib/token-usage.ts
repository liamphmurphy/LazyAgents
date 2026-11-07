export type TokenUsage = {
  input_tokens?: number;
  output_tokens?: number;
  cached_input_tokens?: number;
  uncached_input_tokens?: number;
  unreported_input_tokens?: number;
};

/** Cache reads are part of input, never extra tokens. Missing is not zero. */
export function tokenBreakdown(usage: TokenUsage) {
  const safe = (n: number | undefined) => Number.isFinite(n) ? Math.max(0, n ?? 0) : 0;
  const input = safe(usage.input_tokens);
  const output = safe(usage.output_tokens);
  const hasBreakdown = usage.cached_input_tokens !== undefined && usage.uncached_input_tokens !== undefined;
  const cached = hasBreakdown ? Math.min(input, safe(usage.cached_input_tokens)) : 0;
  const uncached = hasBreakdown ? Math.min(input - cached, safe(usage.uncached_input_tokens)) : 0;
  const unknown = input - cached - uncached;
  return { input, output, cached, uncached, unknown, total: input + output, hasBreakdown: hasBreakdown && (input === 0 || cached + uncached > 0) };
}

export function eventTokenUsage(payload: Record<string, unknown>): TokenUsage {
  const input = Number(payload.input_tokens ?? 0);
  const cached = Number(payload.cached_tokens ?? 0);
  const reported = typeof payload.cache_reported_input_tokens === "number"
    ? payload.cache_reported_input_tokens
    : typeof payload.cached_tokens === "number" ? input : 0;
  const known = Math.max(0, Math.min(input, reported));
  return { input_tokens: input, output_tokens: Number(payload.output_tokens ?? 0), cached_input_tokens: Math.max(0, Math.min(cached, known)), uncached_input_tokens: Math.max(0, known - cached) };
}
