import type { components } from "./api/schema";

export type Board = components["schemas"]["Board"];
export type BoardCard = components["schemas"]["TicketSummary"];
export type BoardStatus = components["schemas"]["TicketStatus"];
export type PendingMove = { card: BoardCard; status: BoardStatus };

export const BOARD_STATUSES: BoardStatus[] = ["backlog", "todo", "in_progress", "in_review", "done"];

/** An overlay, not a cache snapshot: concurrent refetches and other moves survive rollback. */
export function withBoardMoves(board: Board, moves: PendingMove[]): Board {
  const columns = board.columns.map((c) => ({ ...c, cards: [...c.cards] }));
  for (const move of moves) {
    const source = columns.find((c) => c.cards.some((t) => t.key === move.card.key));
    const card = source?.cards.find((t) => t.key === move.card.key) ?? move.card;
    const target = columns.find((c) => c.status === move.status);
    if (!target) continue;
    // If a refetch already sees the target status, do not count the move twice.
    if (source?.status === target.status) continue;
    if (source) {
      source.cards = source.cards.filter((t) => t.key !== card.key);
      source.count = Math.max(0, source.count - 1);
      source.points = Math.max(0, source.points - (card.estimate_points ?? 0));
    }
    target.cards.unshift({ ...card, status: move.status });
    target.count++;
    target.points += card.estimate_points ?? 0;
  }
  return { ...board, columns };
}

/** Only cards with open blockers; counts and points describe what is shown. */
export function onlyBlocked(board: Board): Board {
  return {
    ...board,
    columns: board.columns.map((c) => {
      const cards = c.cards.filter((t) => t.unmet_dependencies > 0);
      return { ...c, cards, count: cards.length, points: cards.reduce((n, t) => n + (t.estimate_points ?? 0), 0), truncated: false };
    }),
  };
}

/** The summary has no revision; a fresh detail supplies it, without overwriting a stale display. */
export function moveRevision(card: BoardCard, detail: { key: string; status: BoardStatus; updated_at: string; revision: number }): number {
  if (detail.key !== card.key || detail.status !== card.status || detail.updated_at !== card.updated_at) {
    throw new Error("Ticket changed elsewhere. The board has been refreshed; review it and retry.");
  }
  return detail.revision;
}
