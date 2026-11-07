"use client";

import { useState } from "react";
import { ApiError, type Ticket, type TicketPatch } from "@/lib/api/client";
import { useRevisionCheckedUpdate } from "@/lib/api/hooks";
import { classifyError } from "@/lib/ticket-draft";
import { Button } from "@/components/ui/button";
import { Banner } from "@/components/ui/primitives";

/** Save one edit on the ticket page; `what` names it in the conflict banner ("Priority → High"). */
export type SaveTicket = (patch: TicketPatch, what: string) => void;

type Unsaved = { patch: TicketPatch; what: string };

/**
 * Every edit on the ticket page goes through the revision-checked PATCH. A
 * stale revision (409) is handled like the edit modal: the latest ticket is
 * shown and the person chooses to reapply their change or keep the latest.
 */
export function useTicketSave(t: Ticket | undefined) {
  const [conflict, setConflict] = useState<Unsaved | null>(null);
  const [error, setError] = useState<string | null>(null);
  const update = useRevisionCheckedUpdate(t?.key ?? "", (e, { patch, what }) => {
    const c = e instanceof ApiError ? classifyError(e.status, e.field, e.message) : { conflict: false, fields: {}, general: e.message };
    // Several queued edits can each hit the 409: keep them all for Reapply.
    if (c.conflict) setConflict((prev) => (prev ? { patch: { ...prev.patch, ...patch }, what: `${prev.what}, ${what}` } : { patch, what }));
    else setError(c.general ?? Object.values(c.fields)[0] ?? e.message);
  });

  const save: SaveTicket = (patch, what) => {
    if (!t) return;
    setConflict(null);
    setError(null);
    update.mutate({ patch, revision: t.revision, what });
  };
  return {
    save,
    conflict,
    error,
    reapply: () => conflict && save(conflict.patch, conflict.what),
    keepLatest: () => setConflict(null),
  };
}

export function ConflictBanner({ what, reloading, onReapply, onKeep }: { what: string; reloading: boolean; onReapply: () => void; onKeep: () => void }) {
  return (
    <div role="alert">
      <Banner
        tone="blocked"
        title="Ticket changed elsewhere"
        actions={
          <>
            <Button variant="outline" disabled={reloading} onClick={onReapply}>
              Reapply mine
            </Button>
            <Button variant="outline" onClick={onKeep}>
              Keep latest
            </Button>
          </>
        }
      >
        Your {what} wasn’t saved. {reloading ? "Loading the latest…" : "Latest is shown."}
      </Banner>
    </div>
  );
}
