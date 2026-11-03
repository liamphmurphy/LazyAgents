"use client";
import Link from "next/link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Ticket } from "@/lib/api/client";
import { hasLiveRun } from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import { TextInput } from "@/components/ui/primitives";
import { StatusGlyph } from "@/components/domain/glyphs";
import { STATUS_NAME } from "@/components/domain/meta";

export function Dependencies({ ticket: t }: { ticket: Ticket }) {
  const [editing, setEditing] = useState(false);
  const [keys, setKeys] = useState<string[]>([]);
  const [revision, setRevision] = useState(t.dependency_revision);
  const [search, setSearch] = useState("");
  const qc = useQueryClient();
  const candidates = useQuery({
    queryKey: ["tickets", "dependency-search", search],
    enabled: editing && search.trim().length >= 2,
    queryFn: () =>
      unwrap(
        api.GET("/tickets", {
          params: { query: { q: search.trim(), view: "all" } },
        }),
      ),
  });
  const save = useMutation({
    mutationFn: () =>
      unwrap(
        api.PUT("/tickets/{key}/dependencies", {
          params: { path: { key: t.key } },
          body: { revision, blocked_by: keys },
        }),
      ),
    onSuccess: () => {
      for (const key of [
        "tickets",
        "planning",
        "inbox",
        "bootstrap",
        "dashboard",
        "agent-overviews",
      ])
        void qc.invalidateQueries({ queryKey: [key] });
      setEditing(false);
    },
  });
  const add = (key: string) => {
    const value = key.trim().toUpperCase();
    if (value && !keys.includes(value)) setKeys([...keys, value]);
    setSearch("");
  };
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="type-heading">Dependencies</h2>
        <Button
          variant="ghost"
          disabled={hasLiveRun(t)}
          title={
            hasLiveRun(t)
              ? "Finish or cancel the active run before changing dependencies"
              : undefined
          }
          onClick={() => {
            setKeys(t.blocked_by.map((d) => d.key));
            setRevision(t.dependency_revision);
            setEditing(true);
            save.reset();
          }}
        >
          Edit
        </Button>
      </div>
      <p className="text-xs text-fg-tertiary">
        A dispatched ticket queues until its blockers are Done. Runs always
        require human approval.
      </p>
      {editing ? (
        <div className="space-y-3 rounded-lg border border-border-strong p-3">
          {keys.map((key) => (
            <div
              key={key}
              className="flex items-center justify-between text-sm"
            >
              <span className="font-mono">{key}</span>
              <Button
                variant="ghost"
                onClick={() => setKeys(keys.filter((k) => k !== key))}
                aria-label={`Remove blocker ${key}`}
              >
                Remove
              </Button>
            </div>
          ))}
          <TextInput
            aria-label="Find a blocker ticket"
            placeholder="Search tickets or enter a ticket key"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (
                e.key === "Enter" &&
                /^[A-Za-z][A-Za-z0-9]*-\d+$/.test(search.trim())
              ) {
                e.preventDefault();
                add(search);
              }
            }}
          />
          {search &&
            candidates.data?.tickets
              .filter((c) => c.key !== t.key && !keys.includes(c.key))
              .slice(0, 8)
              .map((c) => (
                <button
                  key={c.key}
                  type="button"
                  onClick={() => add(c.key)}
                  className="block w-full rounded-md p-2 text-left text-sm hover:bg-hover"
                >
                  <span className="mr-2 font-mono text-xs text-fg-tertiary">
                    {c.key}
                  </span>
                  {c.title}
                </button>
              ))}
          {/^[A-Za-z][A-Za-z0-9]*-\d+$/.test(search.trim()) && (
            <Button variant="outline" onClick={() => add(search)}>
              Add {search.trim().toUpperCase()}
            </Button>
          )}
          {save.error && (
            <p role="alert" className="text-sm text-pill-blocked-fg">
              {save.error.message}
            </p>
          )}
          <div className="flex gap-2">
            <Button
              variant="primary"
              disabled={save.isPending}
              onClick={() => save.mutate()}
            >
              Save dependencies
            </Button>
            <Button
              variant="ghost"
              disabled={save.isPending}
              onClick={() => setEditing(false)}
            >
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <div className="space-y-2">
          {t.blocked_by.length === 0 && (
            <p className="text-sm text-fg-tertiary">No blockers.</p>
          )}
          {t.blocked_by.map((d) => (
            <Link
              key={d.key}
              href={`/tickets/${d.key}`}
              className="flex flex-wrap items-center gap-2 rounded-md border border-border-subtle p-2 text-sm hover:bg-hover"
            >
              <StatusGlyph state={d.status} />
              <span className="font-mono text-xs text-fg-tertiary">
                {d.key}
              </span>
              <span className="min-w-0 flex-1">{d.title}</span>
              <span className="text-xs text-fg-tertiary">
                {STATUS_NAME[d.status]}
              </span>
            </Link>
          ))}
        </div>
      )}
      {t.blocks.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-xs text-fg-tertiary">This ticket blocks</h3>
          {t.blocks.map((d) => (
            <Link
              key={d.key}
              href={`/tickets/${d.key}`}
              className="block text-sm text-accent"
            >
              {d.key} · {d.title}
            </Link>
          ))}
        </div>
      )}
    </section>
  );
}
