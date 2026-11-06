import type { Page, Route } from "@playwright/test";
import type { components } from "../lib/api/schema";

type S = components["schemas"];
const project: S["Project"] = { id: "project-web", key: "WEB", name: "Web app", color: "#6e56cf", default_branch: "main", archived: false, revision: 1 };
const ops: S["Project"] = { ...project, id: "project-ops", key: "OPS", name: "Operations" };
const builder: S["Agent"] = { id: "agent-builder", name: "Builder", handle: "builder", kind: "main", color_index: 0, role: "Developer", specialty: "UI", model: "test", status: "idle" };
const reviewer: S["Agent"] = { ...builder, id: "agent-reviewer", name: "Reviewer", handle: "reviewer", color_index: 2 };
const statuses: S["TicketStatus"][] = ["backlog", "todo", "in_progress", "in_review", "done", "canceled"];
const stamp = (revision: number) => new Date(Date.UTC(2026, 10, 6, 12, 0, revision)).toISOString();

/** In-memory LAZYAGENTS-29 contract, not a substitute for real-backend integration. */
export async function installBoardApi(page: Page) {
  const epics: S["Epic"][] = ["Kanban board", "Other epic"].map((name, i) => ({
    id: `epic-${i + 1}`, project_id: project.id, name, health: "on_track", requires_mockup: false,
    status: "open", criteria_total: 2, criteria_done: 1, points_total: 0, points_done: 0, tickets_total: 0, tickets_open: 0,
  }));
  const criteria: S["EpicCriterion"][] = [
    { id: "criterion-1", epic_id: "epic-1", position: 1, text: "Board shows all statuses", done: true },
    { id: "criterion-2", epic_id: "epic-1", position: 2, text: "Cards can move", done: false },
  ];
  function make(n: number, status: S["TicketStatus"], title: string, extra: Partial<S["Ticket"]> = {}): S["Ticket"] {
    return {
      id: `ticket-${n}`, key: `WEB-${n}`, project, title, status, description_md: "", priority: "medium", labels: [],
      revision: 1, dependency_revision: 1, blocked_by: [], blocks: [], criteria: [], artifacts: [], approvals: [],
      requires_mockup: false, mockup_gate: { required: false, from_epic: false, approved: false, mockups: [] },
      created_at: stamp(1), updated_at: stamp(1), epic: { id: epics[0].id, name: epics[0].name }, ...extra,
    };
  }
  const tickets: S["Ticket"][] = [
    make(1, "todo", "Build ticket modal", { priority: "high", estimate_points: 3, assignee: builder, blocked_by: [{ id: "blocker", key: "WEB-2", title: "Write integration checks", status: "backlog" }] }),
    make(2, "backlog", "Write integration checks", { priority: "none" }),
    make(3, "done", "ADR and contract", { estimate_points: 5, assignee: builder }),
    make(4, "in_progress", "Other epic work", { priority: "urgent", estimate_points: 8, assignee: reviewer, epic: { id: epics[1].id, name: epics[1].name } }),
    make(5, "in_review", "Review board", { priority: "low", estimate_points: 2, assignee: builder }),
    make(6, "canceled", "Canceled task", { estimate_points: 3 }),
  ];
  // Detail criteria are used for the summary progress too.
  tickets[0].criteria = [{ id: "ticket-criterion", position: 1, text: "Works", done: true }];
  const patches: Array<{ key: string; body: S["TicketPatch"] }> = [];
  const boardRequests: string[] = [];
  const unhandled: string[] = [];
  let rejection: { status: number; message: string } | undefined;
  let gate: Promise<void> | undefined;
  let boardError = false;
  function epicSummary(epic: S["Epic"]): S["Epic"] {
    const ts = tickets.filter((t) => t.epic?.id === epic.id && t.status !== "canceled");
    return { ...epic, tickets_total: ts.length, tickets_open: ts.filter((t) => t.status !== "done").length,
      points_total: ts.reduce((n, t) => n + (t.estimate_points ?? 0), 0),
      points_done: ts.filter((t) => t.status === "done").reduce((n, t) => n + (t.estimate_points ?? 0), 0),
      criteria_total: criteria.filter((c) => c.epic_id === epic.id).length,
      criteria_done: criteria.filter((c) => c.epic_id === epic.id && c.done).length };
  }
  function summary(t: S["Ticket"]): S["TicketSummary"] {
    return { id: t.id, key: t.key, project_key: t.project.key, title: t.title, status: t.status, priority: t.priority,
      assignee: t.assignee, epic: t.epic, labels: t.labels, estimate_points: t.estimate_points, updated_at: t.updated_at,
      unmet_dependencies: t.blocked_by.filter((b) => !["done", "canceled"].includes(b.status)).length,
      criteria_total: t.criteria.length, criteria_done: t.criteria.filter((c) => c.done).length };
  }
  async function handle(route: Route) {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const json = (data: unknown, status = 200) => route.fulfill({ status, json: data });
    if (path === "/bootstrap") return json({
      workspace: { id: "workspace", name: "LazyAgents", slug: "lazyagents" },
      me: { id: "user", name: "Liam", email: "liam@example.com", initials: "LM", role_title: "Owner" },
      users: [], agents: [builder, reviewer], epics: epics.map(epicSummary), projects: [project, ops], labels: [],
      counts: { inbox: 0, wiki_suggestions: 0, live_agents: 0 },
    } satisfies S["Bootstrap"]);
    if (path === "/runner") return json({ online: true, slots: 3, busy: 0, queued: 0 });
    if (/^\/projects\/(WEB|OPS)\/board$/.test(path)) {
      boardRequests.push(request.url());
      if (boardError) return json({ code: "internal", message: "Board service unavailable" }, 501);
      const p = path.includes("/OPS/") ? ops : project;
      const epic = epics.find((e) => e.id === url.searchParams.get("epic"));
      const cards = tickets.filter((t) => t.project.key === p.key && (!epic || t.epic?.id === epic.id)
        && (!url.searchParams.get("assignee") || t.assignee?.handle === url.searchParams.get("assignee")));
      return json({ project: p, columns: statuses.filter((s) => s !== "canceled" || url.searchParams.get("include_canceled") === "true").map((status) => {
        const all = cards.filter((t) => t.status === status);
        const capped = status === "done" || status === "canceled";
        return { status, cards: (capped ? all.slice(-50) : all).map(summary), count: all.length,
          points: all.reduce((n, t) => n + (t.estimate_points ?? 0), 0), truncated: capped && all.length > 50 };
      }), ...(epic ? { epic: { epic: epicSummary(epic), criteria: criteria.filter((c) => c.epic_id === epic.id),
        done_blockers: { open_tickets: tickets.filter((t) => t.epic?.id === epic.id && !["done", "canceled"].includes(t.status)).map((t) => t.key),
          unchecked_criteria: criteria.filter((c) => c.epic_id === epic.id && !c.done).map((c) => c.position) } } } : {}) } satisfies S["Board"]);
    }
    const t = tickets.find((t) => path === `/tickets/${t.key}`);
    if (t && request.method() === "GET") return json(t);
    if (t && request.method() === "PATCH") {
      const body = request.postDataJSON() as S["TicketPatch"];
      patches.push({ key: t.key, body });
      const hold = gate; gate = undefined;
      if (hold) await hold;
      const reject = rejection; rejection = undefined;
      if (reject) return json({ code: reject.status === 409 ? "conflict" : "invalid", message: reject.message }, reject.status);
      if (body.revision !== t.revision) return json({ code: "conflict", message: "Revision changed elsewhere" }, 409);
      Object.assign(t, { status: body.status, revision: t.revision + 1, updated_at: stamp(t.revision + 1) });
      return json(t);
    }
    unhandled.push(`${request.method()} ${path}`);
    return json({ code: "not_found", message: `mock: no handler for ${path}` }, 404);
  }
  await page.addInitScript(() => {
    const streams: Array<{ onmessage?: (e: { data: string }) => void }> = [];
    class FakeEventSource {
      onopen?: () => void;
      onmessage?: (e: { data: string }) => void;
      constructor() { streams.push(this); setTimeout(() => this.onopen?.(), 0); }
      close() { const index = streams.indexOf(this); if (index >= 0) streams.splice(index, 1); }
    }
    Object.assign(window, { EventSource: FakeEventSource, __boardStreams: streams });
  });
  await page.route("**/api/v1/**", handle);
  return {
    tickets, criteria, patches, boardRequests, unhandled,
    rejectNext: (status: number, message: string) => { rejection = { status, message }; },
    failBoard: (fail: boolean) => { boardError = fail; },
    holdNextPatch: () => { let release!: () => void; gate = new Promise<void>((resolve) => { release = resolve; }); return release; },
    editTicket: (key: string, patch: Partial<S["Ticket"]>) => {
      const ticket = tickets.find((t) => t.key === key)!;
      Object.assign(ticket, patch, { revision: ticket.revision + 1, updated_at: stamp(ticket.revision + 1) });
    },
    emit: (type: string) => page.evaluate((type) => {
      const streams = (window as unknown as { __boardStreams: Array<{ onmessage?: (e: { data: string }) => void }> }).__boardStreams;
      for (const stream of streams) stream.onmessage?.({ data: JSON.stringify({ type, ids: [] }) });
    }, type),
  };
}
