import { compact } from "@/lib/format";
import { tokenBreakdown, type TokenUsage as Usage } from "@/lib/token-usage";

export function TokenUsage({ usage }: { usage: Usage }) {
  const tokens = tokenBreakdown(usage);
  return <div aria-label="Token breakdown" className="flex flex-wrap items-center gap-x-5 gap-y-1 border-b border-border-subtle px-6 py-2 text-xs text-fg-secondary">
    <span>Input <strong className="font-mono font-medium text-fg">{compact(tokens.input)}</strong></span>
    {tokens.hasBreakdown && <>
      <span>Cached input <strong className="font-mono font-medium text-fg">{compact(tokens.cached)}</strong></span>
      <span>Uncached input <strong className="font-mono font-medium text-fg">{compact(tokens.uncached)}</strong></span>
    </>}
    {tokens.unknown > 0 && <span>Cache breakdown unavailable for <strong className="font-mono font-medium">{compact(tokens.unknown)}</strong> input</span>}
    <span>Output <strong className="font-mono font-medium text-fg">{compact(tokens.output)}</strong></span>
    <span className="text-fg-tertiary">Total includes cached input.</span>
  </div>;
}
