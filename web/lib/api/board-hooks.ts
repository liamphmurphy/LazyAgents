"use client";

import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "./client";
import { keys } from "./hooks";
import { moveRevision, type BoardCard, type BoardStatus, type PendingMove } from "../board";

export type BoardFilters = { epic?: string; assignee?: string; include_canceled: boolean };

export function useBoard(project: string, filters: BoardFilters) {
  return useQuery({
    queryKey: ["boards", project, filters],
    enabled: !!project,
    queryFn: ({ signal }) => unwrap(api.GET("/projects/{key}/board", {
      params: { path: { key: project }, query: filters }, signal,
    })),
  });
}

/** Mount once per filter view. Requests finishing after navigation only invalidate shared caches. */
export function useBoardMoves() {
  const qc = useQueryClient();
  const locks = useRef(new Set<string>());
  const [pending, setPending] = useState<Record<string, PendingMove>>({});
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();

  async function move(card: BoardCard, status: BoardStatus) {
    if (card.status === status || locks.current.has(card.key)) return;
    locks.current.add(card.key);
    setError(undefined);
    setNotice(undefined);
    setPending((p) => ({ ...p, [card.key]: { card, status } }));
    try {
      const detail = await unwrap(api.GET("/tickets/{key}", { params: { path: { key: card.key } } }));
      const revision = moveRevision(card, detail);
      const updated = await unwrap(api.PATCH("/tickets/{key}", {
        params: { path: { key: card.key } }, body: { status, revision },
      }));
      qc.setQueryData(keys.ticket(card.key), updated);
      setNotice(`Moved ${card.key}.`);
    } catch (e) {
      setError(`Couldn't move ${card.key}: ${e instanceof Error ? e.message : "Request failed. Try again."}`);
      setPending((p) => {
        const next = { ...p };
        delete next[card.key];
        return next;
      });
    } finally {
      // Refetch on success or failure; never restore a snapshot over newer SSE data.
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["boards"] }),
        qc.invalidateQueries({ queryKey: keys.tickets }),
        qc.invalidateQueries({ queryKey: keys.bootstrap }),
        qc.invalidateQueries({ queryKey: ["epics"] }),
      ]);
      locks.current.delete(card.key);
      setPending((p) => {
        const next = { ...p };
        delete next[card.key];
        return next;
      });
    }
  }

  return { pending, move, error, notice, dismissError: () => setError(undefined) };
}
