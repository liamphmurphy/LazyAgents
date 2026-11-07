"use client";

import { useQuery } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";
import { RUNS_PAGE } from "./hooks";

// Each query sits under a prefix the shell's SSE subscription invalidates
// (runs, chats, boards, tickets), so the panels refresh without a reload.

/** Upper bound on pages read per panel; counts past it are shown as "N+". */
const MAX_PAGES = 20;

/** Every live top-level run, across pages. */
export function useDashboardLiveRuns() {
  return useQuery({
    queryKey: ["runs", "dashboard-live"] as const,
    queryFn: async ({ signal }) => {
      const runs: Schemas["Run"][] = [];
      for (let page = 0; page < MAX_PAGES; page++) {
        const d = await unwrap(api.GET("/runs", { params: { query: { state: "live", offset: page * RUNS_PAGE } }, signal }));
        runs.push(...d.runs);
        if (!d.has_more) return { runs, complete: true };
      }
      return { runs, complete: false };
    },
    // Elapsed time and cost tick between events.
    refetchInterval: 30_000,
  });
}

/** Unarchived chats in every project, most recently active first. */
export function useDashboardChats() {
  return useQuery({
    queryKey: ["chats", "dashboard"] as const,
    queryFn: async ({ signal }) => {
      const chats: Schemas["ChatSession"][] = [];
      for (let page = 0; page < MAX_PAGES; page++) {
        const d = await unwrap(api.GET("/chats", { params: { query: { archived: false, offset: chats.length } }, signal }));
        chats.push(...d.chats);
        if (!d.has_more || !d.chats.length) return { chats, complete: true };
      }
      return { chats, complete: false };
    },
  });
}

/**
 * Board cards of every active project. Boards return open columns complete
 * (only done and canceled are truncated), unlike /tickets which stops at 500,
 * so no blocked ticket is missed.
 */
export function useDashboardBoards(projects: string[] | undefined) {
  return useQuery({
    queryKey: ["boards", "dashboard", projects] as const,
    enabled: !!projects,
    queryFn: async ({ signal }) => {
      const boards = await Promise.all(
        (projects ?? []).map((key) =>
          unwrap(api.GET("/projects/{key}/board", { params: { path: { key }, query: { include_canceled: false } }, signal })),
        ),
      );
      return boards.flatMap((b) => b.columns.flatMap((c) => c.cards));
    },
  });
}

/** What blocks each previewed ticket. Under ["tickets"]: a blocker finishing refreshes it. */
export function useBlockers(keys: string[]) {
  return useQuery({
    queryKey: ["tickets", "dashboard-blockers", keys] as const,
    enabled: keys.length > 0,
    queryFn: async ({ signal }) => {
      const details = await Promise.all(keys.map((key) => unwrap(api.GET("/tickets/{key}", { params: { path: { key } }, signal }))));
      return Object.fromEntries(details.map((t) => [t.key, t.blocked_by.filter((b) => b.status !== "done" && b.status !== "canceled")]));
    },
  });
}
