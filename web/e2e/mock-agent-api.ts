import type { Page, Route } from "@playwright/test";

/**
 * In-memory stand-in for the agent REST contract (api/openapi.yaml, tag `agents`):
 * GET /agents/{handle}, PUT /agents/{handle}/definition,
 * PUT /agents/{handle}/overrides/{project}, GET /agents/{handle}/resolved.
 * State is kept across page reloads. A definition whose harness is opencode
 * must have a provider-qualified model, like the server (agentdef/opencode.go).
 */

type Harness = "claude-code" | "codex" | "opencode";
type Spec = {
  harness: Harness; model: string; instructions: string; scopes: Record<string, string>;
  budget: Record<string, unknown>; max_concurrency: number; mcp_servers: unknown[];
  harness_options: Record<string, Record<string, string>>; fallbacks?: Array<{ harness: Harness; model: string }>;
};
type Override = { harness?: Harness; model?: string; [k: string]: unknown };

const HARNESSES = [
  { name: "claude-code", label: "Claude Code", tool_gating: true, permission_prompt: true, native_cost: true, resume: true },
  { name: "codex", label: "Codex", tool_gating: false, permission_prompt: false, native_cost: true, resume: true },
  { name: "opencode", label: "OpenCode", tool_gating: true, permission_prompt: true, native_cost: false, resume: true },
];
const PROJECT = { id: "00000000-0000-4000-8000-0000000000a1", key: "WEB", name: "Web app", color: "#6e56cf", default_branch: "main", archived: false, revision: 1 };
const UUID = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;

export type Saved = { definition: Array<Record<string, unknown>>; overrides: Array<{ project: string; body: Override }> };

export async function installAgentApi(page: Page) {
  const now = () => new Date().toISOString();
  const agent = { id: UUID(1), kind: "main", name: "Atlas", handle: "atlas", color_index: 0, role: "Backend", specialty: "APIs", model: "opus-5.5", status: "idle" };
  let spec: Spec = {
    harness: "claude-code", model: "opus-5.5", instructions: "Be helpful.", scopes: { "repo:read": "allowed" },
    budget: {}, max_concurrency: 1, mcp_servers: [], harness_options: {},
  };
  let version = 1;
  let override: Override | undefined;
  const unhandled: string[] = [];
  const saved: Saved = { definition: [], overrides: [] };

  const detail = () => ({
    agent: { ...agent, model: spec.model },
    specialty: agent.specialty,
    definition: { id: UUID(100 + version), version, note: "", created_at: now() },
    spec,
    versions: [{ id: UUID(100 + version), version, note: "", created_at: now() }],
    overrides: override ? [{ project: PROJECT, override, changes: Object.keys(override).map((k) => `${k} = ${String(override![k])}`), version: 1, updated_at: now() }] : [],
    harnesses: HARNESSES,
    catalog: [{ scope: "repo:read", description: "Read the repo", brokered: false }],
  });

  async function handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const method = req.method();
    const json = (data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
    const body = () => (req.postDataJSON() ?? {}) as Record<string, unknown>;

    if (path === "/bootstrap") {
      return json({
        workspace: { id: UUID(2), name: "Acme", slug: "acme" },
        me: { id: UUID(3), name: "Liam", email: "liam@example.com", initials: "L", role_title: "Owner" },
        users: [], agents: [agent], epics: [], labels: [], projects: [PROJECT],
        counts: { inbox: 0, wiki_suggestions: 0, live_agents: 0 },
      });
    }
    if (path === "/runner") return json({ online: true, runners: [], slots: 1, busy: 0, queued: 0 });
    if (path === "/saved-views" && method === "GET") return json({ views: [] });
    if (path === "/agents/atlas" && method === "GET") return json(detail());
    if (path === "/agents/atlas/definition" && method === "PUT") {
      const b = body();
      saved.definition.push(b);
      const s = b.spec as Spec;
      if (s.harness === "opencode" && !/^[^/\s]+\/[^\s]+$/.test(s.model)) {
        return json({ code: "invalid", field: "model", message: `opencode model "${s.model}" must be provider-qualified (provider/model)` }, 422);
      }
      spec = s;
      version++;
      return json(detail());
    }
    if (path === "/agents/atlas/overrides/WEB" && method === "PUT") {
      const b = body() as Override;
      saved.overrides.push({ project: "WEB", body: b });
      override = Object.keys(b).some((k) => b[k] !== undefined && !(Array.isArray(b[k]) && (b[k] as unknown[]).length === 0)) ? b : undefined;
      return json(detail());
    }
    if (path === "/agents/atlas/resolved") {
      return json({
        harness: override?.harness ?? spec.harness, model: override?.model ?? spec.model, instructions: spec.instructions,
        scopes: [], budget: {}, max_concurrency: 1, mcp_servers: [], harness_options: {}, setup: [],
        provenance: { ...(override?.harness ? { harness: "project" } : {}), ...(override?.model ? { model: "project" } : {}) },
      });
    }
    unhandled.push(`${method} ${path}`);
    return json({ code: "not_found", message: `mock: no handler for ${method} ${path}` }, 404);
  }

  await page.addInitScript(() => {
    class FakeEventSource {
      onopen?: () => void;
      onerror?: () => void;
      onmessage?: (e: { data: string }) => void;
      constructor() {
        setTimeout(() => this.onopen?.(), 0);
      }
      close() {}
    }
    (window as unknown as { EventSource: unknown }).EventSource = FakeEventSource;
  });
  await page.route("**/api/v1/**", handle);
  return { unhandled, saved };
}
