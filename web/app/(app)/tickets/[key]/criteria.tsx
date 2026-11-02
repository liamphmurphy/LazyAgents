"use client";

import { useState } from "react";
import type { Ticket } from "@/lib/api/client";
import { useAddCriterion, useDeleteCriterion, useUpdateCriterion } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { IconButton } from "@/components/ui/button";

export function Criteria({ t }: { t: Ticket }) {
  const add = useAddCriterion(t.key);
  const update = useUpdateCriterion(t.key);
  const remove = useDeleteCriterion(t.key);
  const [draft, setDraft] = useState("");
  const done = t.criteria.filter((c) => c.done).length;

  return (
    <section className="flex flex-col gap-2">
      <h2 className="flex items-baseline gap-2 type-heading">
        Acceptance criteria
        {t.criteria.length > 0 && (
          <span className="font-mono text-xs font-normal text-fg-tertiary">
            {done}/{t.criteria.length}
          </span>
        )}
      </h2>
      <ul className="flex flex-col">
        {t.criteria.map((c) => (
          <li key={c.id} className="group flex min-h-8 items-center gap-3 rounded-md">
            <input
              id={`crit-${c.id}`}
              type="checkbox"
              checked={c.done}
              onChange={(e) => update.mutate({ id: c.id, done: e.target.checked })}
              className="size-3.5 flex-none accent-[var(--state-done)]"
            />
            <label htmlFor={`crit-${c.id}`} className={cx("flex-1 text-[13.5px]", c.done ? "text-fg-tertiary line-through" : "text-fg-2")}>
              {c.text}
            </label>
            <IconButton
              icon="x"
              label={`Remove criterion: ${c.text}`}
              size={24}
              onClick={() => remove.mutate(c.id)}
              className="opacity-0 focus-visible:opacity-100 group-hover:opacity-100"
            />
          </li>
        ))}
      </ul>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!draft.trim()) return;
          add.mutate(draft, { onSuccess: () => setDraft("") });
        }}
        className="flex items-center gap-3"
      >
        <span aria-hidden className="size-3.5 flex-none rounded-[3px] border border-dashed border-border-strong" />
        <label htmlFor="new-criterion" className="sr-only">
          Add a criterion
        </label>
        <input
          id="new-criterion"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Add a criterion and press Enter"
          className="h-8 flex-1 bg-transparent text-[13.5px] text-fg outline-none placeholder:text-fg-tertiary"
        />
      </form>
      {add.isError && <p className="text-xs text-pill-blocked-fg">{add.error.message}</p>}
    </section>
  );
}
