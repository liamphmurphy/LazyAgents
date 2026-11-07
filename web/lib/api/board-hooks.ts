"use client";

import { useCallback, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, api, unwrap, type TicketPatch } from "./client";
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
  // A 409 (or a card that went stale before PATCH) is shown as a toast after the refetch.
  const [conflict, setConflict] = useState<{ ticket: string; message: string }>();
  const [notice, setNotice] = useState<string>();
  const dismissConflict = useCallback(() => setConflict(undefined), []);

  async function move(card: BoardCard, status: BoardStatus) {
    if (card.status === status || locks.current.has(card.key)) return;
    locks.current.add(card.key);
    setError(undefined);
    setConflict(undefined);
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
      const stale = (e instanceof ApiError && e.status === 409) || (e instanceof Error && !(e instanceof ApiError) && e.message.startsWith("Ticket changed elsewhere"));
      if (stale) setConflict({ ticket: card.key, message: e instanceof Error ? e.message : "" });
      else setError(`Couldn't move ${card.key}: ${e instanceof Error ? e.message : "Request failed. Try again."}`);
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

  return {
    pending, move, error, notice, conflict,
    dismissError: () => setError(undefined),
    dismissConflict,
  };
}

/** Fresh detail for the editor. gcTime 0 so every open fetches again instead of showing a cached copy. */
export function useTicketEditorDetail(key: string | undefined) {
  return useQuery({
    enabled: !!key,
    queryKey: ["tickets", "editor", key ?? ""] as const,
    gcTime: 0,
    retry: false,
    queryFn: ({ signal }) => unwrap(api.GET("/tickets/{key}", { params: { path: { key: key! } }, signal })),
  });
}

/** Saves one draft PATCH, then waits for board/ticket caches to refetch so the card shows it. */
export function useSaveTicketDraft(key: string) {
  const qc = useQueryClient();
  return async (patch: TicketPatch) => {
    const saved = await unwrap(api.PATCH("/tickets/{key}", { params: { path: { key } }, body: patch }));
    qc.setQueryData(keys.ticket(key), saved);
    void qc.invalidateQueries({ queryKey: keys.activity(key) });
    void qc.invalidateQueries({ queryKey: ["knowledge"] });
    await Promise.all([
      qc.invalidateQueries({ queryKey: ["boards"] }),
      qc.invalidateQueries({ queryKey: keys.tickets }),
      qc.invalidateQueries({ queryKey: keys.bootstrap }),
      qc.invalidateQueries({ queryKey: ["epics"] }),
    ]);
    return saved;
  };
}
