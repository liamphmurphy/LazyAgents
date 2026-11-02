import type { AgentDetail, Harness, ScopeMode } from "@/lib/api/client";

export type Enforcement = "credential" | "brokered" | "harness" | "none";

/**
 * Mirrors server/internal/agentdef/enforce.go EnforcementFor, so an unsaved
 * draft shows how each scope would hold. The server stays authoritative
 * (GET /agents/{handle}/resolved).
 */
export function enforcementFor(
  scope: string,
  mode: ScopeMode,
  harness: Harness,
  d: Pick<AgentDetail, "catalog" | "harnesses">,
): { enforcement: Enforcement; how: string } {
  const info = d.catalog.find((c) => c.scope === scope);
  const caps = d.harnesses.find((h) => h.name === harness);
  if (info?.api_only) {
    if (mode === "allowed") return { enforcement: "brokered", how: "Edits go through LazyAgents and publish directly" };
    if (mode === "requires_approval") return { enforcement: "brokered", how: "Edits are saved as suggestions a person accepts" };
    return { enforcement: "brokered", how: "Only the agent's own memory pages can be written" };
  }
  if (mode === "requires_approval" && info?.brokered) return { enforcement: "brokered", how: "Runs server-side after a person approves" };
  if (mode === "requires_approval") {
    return caps?.permission_prompt
      ? { enforcement: "harness", how: "Each use prompts for approval through LazyAgents" }
      : { enforcement: "none", how: "This harness can't pause for approval; the agent is told to ask first" };
  }
  if (info?.secret && mode === "denied") return { enforcement: "credential", how: `The ${info.secret} secret is never injected` };
  if (info?.secret) return { enforcement: "credential", how: `The ${info.secret} secret is injected into the container` };
  if (caps?.tool_gating && mode === "denied") return { enforcement: "harness", how: "Matching tools are disallowed" };
  if (caps?.tool_gating) return { enforcement: "harness", how: "Matching tools are allowed" };
  return { enforcement: "none", how: "Sandbox mode only; the agent is instructed" };
}

export const MODE_RANK: Record<ScopeMode, number> = { allowed: 0, requires_approval: 1, denied: 2 };
export const MODE_LABEL: Record<ScopeMode, string> = { allowed: "Allowed", requires_approval: "Approval", denied: "Denied" };
