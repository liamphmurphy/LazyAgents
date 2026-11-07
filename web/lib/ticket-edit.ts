import type { components } from "./api/schema";

type Ticket = components["schemas"]["Ticket"];
type TicketPatch = components["schemas"]["TicketPatch"];

/** v1 estimate scale (tracker.EstimateScale); the server answers 422 for anything else. */
export const ESTIMATE_SCALE = [1, 2, 3, 5, 8, 13] as const;

/** The fields the edit modal saves through one revision-checked PATCH. */
export type EditForm = {
  title: string;
  description_md: string;
  status: Ticket["status"];
  priority: Ticket["priority"];
  estimate_points: number | null;
  assignee_agent_id: string | null;
  epic_id: string | null;
};

export type EditField = keyof EditForm;

export const FIELD_LABEL: Record<EditField, string> = {
  title: "title",
  description_md: "description",
  status: "status",
  priority: "priority",
  estimate_points: "estimate",
  assignee_agent_id: "assignee",
  epic_id: "epic",
};

const FIELDS = Object.keys(FIELD_LABEL) as EditField[];

export function formOf(t: Ticket): EditForm {
  return {
    title: t.title,
    description_md: t.description_md,
    status: t.status,
    priority: t.priority,
    estimate_points: t.estimate_points ?? null,
    assignee_agent_id: t.assignee?.id ?? null,
    epic_id: t.epic?.id ?? null,
  };
}

/** Fields of `form` that differ from `base`: the person's unsaved edits. */
export function changesOf(base: EditForm, form: EditForm): Partial<EditForm> {
  const out: Partial<EditForm> = {};
  for (const f of FIELDS) if (form[f] !== base[f]) (out as Record<string, unknown>)[f] = form[f];
  return out;
}

/** One PATCH carrying only the changed fields plus the revision they were based on. */
export function patchOf(changes: Partial<EditForm>, revision: number): TicketPatch {
  return { ...changes, revision };
}

/** Reapply the person's edits on top of the latest server version after a 409. */
export function reapply(latest: EditForm, mine: Partial<EditForm>): EditForm {
  return { ...latest, ...mine };
}

/** Maps a 422 `field` from the API onto a form field, if it is one. */
export function errorField(field: string | undefined): EditField | undefined {
  return field && (FIELDS as string[]).includes(field) ? (field as EditField) : undefined;
}
