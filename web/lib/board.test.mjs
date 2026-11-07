import test from "node:test";
import assert from "node:assert/strict";
import { withBoardMoves, moveRevision, onlyBlocked } from "./board.ts";

const card = { key: "WEB-1", status: "todo", updated_at: "2026-11-06T12:00:00Z", estimate_points: 3 };
const other = { ...card, key: "WEB-2", estimate_points: undefined };
const board = { columns: [
  { status: "todo", cards: [card, other], count: 2, points: 3, truncated: false },
  { status: "in_progress", cards: [], count: 0, points: 0, truncated: false },
  { status: "done", cards: [], count: 50, points: 100, truncated: true },
] };

test("moves change counts and points without mutating server data or losing truncation totals", () => {
  const next = withBoardMoves(board, [{ card, status: "done" }]);
  assert.equal(next.columns[0].count, 1);
  assert.equal(next.columns[0].points, 0);
  assert.equal(next.columns[2].count, 51);
  assert.equal(next.columns[2].points, 103);
  assert.equal(next.columns[2].cards[0].status, "done");
  assert.equal(board.columns[0].cards.length, 2);
  assert.equal(board.columns[2].count, 50);
});

test("overlapping moves rollback independently and preserve refetched card changes", () => {
  const refreshed = structuredClone(board);
  refreshed.columns[0].cards[0].title = "Agent changed title";
  const next = withBoardMoves(refreshed, [{ card, status: "done" }, { card: other, status: "in_progress" }]);
  assert.equal(next.columns[2].cards[0].title, "Agent changed title");
  const rolledBack = withBoardMoves(refreshed, [{ card: other, status: "in_progress" }]);
  assert.equal(rolledBack.columns[0].cards[0].key, "WEB-1");
  assert.equal(rolledBack.columns[1].points, 0);
  assert.equal(rolledBack.columns[1].count, 1);
});

test("a refetch which has already applied the move is not moved or counted twice", () => {
  const moved = withBoardMoves(board, [{ card, status: "done" }]);
  assert.deepEqual(withBoardMoves(moved, [{ card, status: "done" }]), moved);
});

test("only a matching detail supplies a move revision", () => {
  assert.equal(moveRevision(card, { ...card, revision: 9 }), 9);
  for (const change of [{ status: "done" }, { updated_at: "new" }, { key: "WEB-2" }]) {
    assert.throws(() => moveRevision(card, { ...card, revision: 10, ...change }), /changed elsewhere/);
  }
});

test("blocked-only keeps cards with open blockers and recomputes counts and points", () => {
  const card = (key, unmet, points) => ({ key, unmet_dependencies: unmet, estimate_points: points });
  const board = { project: {}, columns: [
    { status: "todo", cards: [card("A-1", 1, 3), card("A-2", 0, 5), card("A-3", 2, null)], count: 3, points: 8, truncated: false },
    { status: "done", cards: [card("A-4", 0, 2)], count: 60, points: 120, truncated: true },
  ] };
  const out = onlyBlocked(board);
  assert.deepEqual(out.columns[0].cards.map((c) => c.key), ["A-1", "A-3"]);
  assert.equal(out.columns[0].count, 2);
  assert.equal(out.columns[0].points, 3);
  assert.deepEqual([out.columns[1].count, out.columns[1].points, out.columns[1].truncated], [0, 0, false]);
});
