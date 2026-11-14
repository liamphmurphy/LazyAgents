"use client";

import { useState } from "react";
import { useSearchParams } from "next/navigation";
import { useCreateSavedView, useSavedViews } from "@/lib/api/hooks";
import { updateWorkUrl } from "@/lib/work-navigation";
import { FilterMenu } from "@/components/domain/pickers";
import { Button } from "@/components/ui/button";
import { TextInput } from "@/components/ui/primitives";

export function WorkSavedViews() {
  const params = useSearchParams();
  const views = useSavedViews();
  const create = useCreateSavedView();
  const [saving, setSaving] = useState(false);
  const [name, setName] = useState("");
  function apply(id: string) {
    const v = views.data?.find((v) => v.id === id);
    if (!v) return;
    const patch: Record<string, string | null> = { saved: id };
    for (const key of [
      "status",
      "label",
      "q",
      "filter_epic",
      "agent",
      "ticket_view",
      "ready",
      "blocked",
      "unestimated",
    ])
      patch[key] = null;
    for (const [key, value] of new URLSearchParams(v.query)) {
      if (key === "view") patch.ticket_view = value;
      else if (key === "assignee") patch.agent = value;
      else if (key === "epic") patch.filter_epic = value;
      else if (
        [
          "status",
          "label",
          "q",
          "layout",
          "ready",
          "blocked",
          "unestimated",
        ].includes(key)
      )
        patch[key] = value;
    }
    updateWorkUrl(patch);
  }
  return (
    <div className="flex flex-wrap items-center gap-1 text-11">
      <FilterMenu
        quiet
        label="Saved views"
        value={params.get("saved") ?? ""}
        onChange={apply}
        options={[
          { value: "", label: "Saved views" },
          ...(views.data ?? []).map((v) => ({ value: v.id, label: v.name })),
        ]}
      />
      <button
        aria-label="Save current Work filters"
        className="px-2 text-fg-tertiary hover:text-fg"
        onClick={() => setSaving(!saving)}
      >
        +
      </button>
      {views.isError && (
        <p role="alert">
          Couldn’t load saved views.{" "}
          <button onClick={() => views.refetch()}>Retry</button>
        </p>
      )}
      {saving && (
        <form
          className="flex flex-wrap gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (!name.trim() || create.isPending) return;
            const q = new URLSearchParams();
            for (const key of [
              "status",
              "label",
              "q",
              "layout",
              "ready",
              "blocked",
              "unestimated",
            ])
              if (params.has(key)) q.set(key, params.get(key)!);
            if (params.has("filter_epic"))
              q.set("epic", params.get("filter_epic")!);
            if (params.has("agent")) q.set("assignee", params.get("agent")!);
            q.set("view", params.get("ticket_view") ?? "all");
            create.mutate(
              { name: name.trim(), query: q.toString() },
              {
                onSuccess: () => {
                  setSaving(false);
                  setName("");
                },
              },
            );
          }}
        >
          <TextInput
            aria-label="View name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={80}
          />
          <Button type="submit" disabled={!name.trim() || create.isPending}>
            Save
          </Button>
          <Button onClick={() => setSaving(false)}>Cancel</Button>
          {create.isError && <p role="alert">{create.error.message}</p>}
        </form>
      )}
    </div>
  );
}
