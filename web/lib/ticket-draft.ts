// Pure draft/diff logic for the board ticket editor (ADR-034). No imports so
// node can run it directly in unit tests.

export const ESTIMATE_SCALE = [1, 2, 3, 5, 8, 13] as const;

export type CriterionSnapshot = { id: string; position: number; text: string; done: boolean };
export type DraftCriterion = { uid: string; id?: string; text: string; done: boolean };

export type TicketSnapshot = {
  revision: number;
  title: string;
  description_md: string;
  status: string;
  priority: string;
  estimate_points: number | null;
  assignee_agent_id: string | null;
  epic_id: string | null;
  criteria: CriterionSnapshot[];
};

export type TicketDraft = Omit<TicketSnapshot, "revision" | "criteria"> & { criteria: DraftCriterion[] };

export type DraftPatch = {
  revision: number;
  title?: string;
  description_md?: string;
  status?: string;
  priority?: string;
  estimate_points?: number | null;
  assignee_agent_id?: string | null;
  epic_id?: string | null;
  criteria?: { expected: CriterionSnapshot[]; items: Array<{ id?: string; text: string; done: boolean }> };
};

type DetailLike = {
  revision: number;
  title: string;
  description_md: string;
  status: string;
  priority: string;
  estimate_points?: number | null;
  assignee?: { id: string } | null;
  epic?: { id: string } | null;
  criteria: CriterionSnapshot[];
};

let nextUid = 0;
export const newCriterion = (): DraftCriterion => ({ uid: `new-${++nextUid}`, text: "", done: false });

/** Immutable baseline taken from a ticket detail when the editor opens. */
export function snapshotFromTicket(t: DetailLike): TicketSnapshot {
  return {
    revision: t.revision,
    title: t.title,
    description_md: t.description_md,
    status: t.status,
    priority: t.priority,
    estimate_points: t.estimate_points ?? null,
    assignee_agent_id: t.assignee?.id ?? null,
    epic_id: t.epic?.id ?? null,
    criteria: t.criteria.map((c) => ({ id: c.id, position: c.position, text: c.text, done: c.done })),
  };
}

export function draftFromSnapshot(s: TicketSnapshot): TicketDraft {
  const { revision: _revision, criteria, ...fields } = s;
  void _revision;
  return { ...fields, criteria: criteria.map((c) => ({ uid: c.id, id: c.id, text: c.text, done: c.done })) };
}

/** New rows with blank text are dropped; they carry no information. */
function effectiveItems(draft: TicketDraft) {
  return draft.criteria
    .filter((c) => c.id !== undefined || c.text.trim() !== "")
    .map((c) => ({ ...(c.id !== undefined && { id: c.id }), text: c.text, done: c.done }));
}

export function criteriaChanged(base: TicketSnapshot, draft: TicketDraft): boolean {
  const items = effectiveItems(draft);
  if (items.length !== base.criteria.length) return true;
  return items.some((it, i) => it.id !== base.criteria[i].id || it.text !== base.criteria[i].text || it.done !== base.criteria[i].done);
}

/** The single PATCH body: changed fields plus revision, or null when nothing changed. */
export function buildPatch(base: TicketSnapshot, draft: TicketDraft): DraftPatch | null {
  const patch: DraftPatch = { revision: base.revision };
  let changed = false;
  const scalars = ["title", "description_md", "status", "priority", "estimate_points", "assignee_agent_id", "epic_id"] as const;
  for (const k of scalars) {
    if (draft[k] !== base[k]) {
      (patch as Record<string, unknown>)[k] = draft[k];
      changed = true;
    }
  }
  if (criteriaChanged(base, draft)) {
    patch.criteria = { expected: base.criteria, items: effectiveItems(draft) };
    changed = true;
  }
  return changed ? patch : null;
}

export function moveCriterion(list: DraftCriterion[], index: number, delta: -1 | 1): DraftCriterion[] {
  const to = index + delta;
  if (to < 0 || to >= list.length) return list;
  const next = [...list];
  [next[index], next[to]] = [next[to], next[index]];
  return next;
}

const FIELD_ERRORS = ["title", "description_md", "status", "priority", "estimate_points", "epic_id", "assignee_agent_id", "criteria"] as const;
export type EditorErrors = { conflict: boolean; fields: Record<string, string>; general?: string };

/** Maps an API failure to the editor's error state (stale banner, field error, or general). */
export function classifyError(status: number, field: string | undefined, message: string): EditorErrors {
  if (status === 409 && (field === "revision" || field === "criteria")) return { conflict: true, fields: {} };
  if (status === 422 && field && (FIELD_ERRORS as readonly string[]).includes(field)) return { conflict: false, fields: { [field]: message } };
  return { conflict: false, fields: {}, general: message };
}
