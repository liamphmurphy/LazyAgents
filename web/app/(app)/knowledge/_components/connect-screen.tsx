"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { cx } from "@/lib/cx";
import { pageHref } from "@/lib/knowledge";
import { useCreatePage, useNetwork, usePages } from "@/lib/api/knowledge-hooks";
import { useRunScheduledTask, useSaveScheduledTask, useScheduledTasks } from "@/lib/api/connector-hooks";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Skeleton } from "@/components/ui/primitives";
import { StatusBar } from "./kit";
import { useKnowledgeTab } from "./shell";

const TAB = { id: "connect", title: "Keep notes connected", kind: "connect" as const };
const LIBRARIAN = "Weekly Knowledge librarian";
const LIBRARIAN_PROMPT =
  "Call librarian_pass with all: true. It proposes links for unlinked mentions and reports orphans and unresolved links; every change arrives as a suggestion for me to review. Then summarise what it found in a few lines.";

type Tab = "unresolved" | "orphans";

/**
 * The upkeep side of a Zettelkasten: links that point at pages that don't exist
 * yet, pages nothing links to, and a persona's weekly pass that proposes the
 * links you missed. Unlinked mentions of a page are on its Links panel.
 */
export function ConnectScreen() {
  useKnowledgeTab(TAB);
  const router = useRouter();
  const [tab, setTab] = useState<Tab>("unresolved");
  const net = useNetwork(false);
  const orphans = usePages({ orphans: true, limit: 100 });
  const create = useCreatePage();

  const unresolved = useMemo(() => {
    const titles = new Map((net.data?.nodes ?? []).map((n) => [n.id, n]));
    const by = new Map<string, { name: string; from: { id: string; title: string }[] }>();
    for (const l of net.data?.links ?? []) {
      if (!l.unresolved) continue;
      const key = l.unresolved.toLowerCase();
      const e = by.get(key) ?? { name: l.unresolved, from: [] };
      const src = titles.get(l.source);
      if (src && !e.from.some((f) => f.id === src.id)) e.from.push({ id: src.id, title: src.title });
      by.set(key, e);
    }
    return [...by.values()].sort((a, b) => b.from.length - a.from.length || a.name.localeCompare(b.name));
  }, [net.data]);

  const counts: Record<Tab, number | undefined> = { unresolved: net.data ? unresolved.length : undefined, orphans: orphans.data?.rows.length };

  return (
    <>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[1000px] px-8 pb-16 pt-8">
          <h1 className="flex items-center gap-3 text-[26px] font-bold tracking-[-0.02em] text-fg">
            <Icon name="link" size={20} className="text-accent" /> Keep notes connected
          </h1>
          <Librarian />
          <div role="tablist" className="mt-8 flex gap-7 border-b border-border-subtle">
            {(
              [
                ["unresolved", "Unresolved links"],
                ["orphans", "Orphans"],
              ] as const
            ).map(([t, label]) => (
              <button key={t} role="tab" aria-selected={tab === t} onClick={() => setTab(t)} className={cx("-mb-px flex items-center gap-2 border-b-2 pb-2.5 text-sm", tab === t ? "border-accent font-semibold text-fg" : "border-transparent text-fg-tertiary hover:text-fg-secondary")}>
                {label}
                {counts[t] !== undefined && <span className="rounded-full bg-selected px-1.5 text-[10px] font-semibold text-fg-secondary">{counts[t]}</span>}
              </button>
            ))}
          </div>
          {tab === "unresolved" && (
            <>
              <p className="mt-4 text-[13px] text-fg-tertiary">Links to pages that don&apos;t exist yet. Create one and every link to it starts working.</p>
              <ul className="mt-4 space-y-2">
                {net.isPending && <Skeleton className="h-12 w-full" />}
                {unresolved.map((u) => (
                  <li key={u.name} className="flex items-center gap-4 rounded-xl border border-border-subtle bg-surface px-4 py-3">
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-[#8E86C8] underline decoration-dotted underline-offset-4">{u.name}</span>
                      <span className="mt-0.5 block truncate text-xs text-fg-tertiary">
                        from{" "}
                        {u.from.slice(0, 3).map((f, i) => (
                          <span key={f.id}>
                            {i > 0 && ", "}
                            <Link href={pageHref(f.id, f.title)} className="hover:text-fg-secondary hover:underline">
                              {f.title}
                            </Link>
                          </span>
                        ))}
                        {u.from.length > 3 && ` and ${u.from.length - 3} more`}
                      </span>
                    </span>
                    <span className="text-xs text-fg-tertiary">
                      {u.from.length} {u.from.length === 1 ? "page" : "pages"}
                    </span>
                    <Button onClick={() => create.mutate({ title: u.name.split("/").pop() ?? u.name, path: u.name, body_md: "" }, { onSuccess: (p) => router.push(pageHref(p.id, p.title)) })} disabled={create.isPending}>
                      Create page
                    </Button>
                  </li>
                ))}
                {net.data && !unresolved.length && <li className="py-8 text-center text-[13px] text-fg-tertiary">Every link resolves.</li>}
              </ul>
            </>
          )}
          {tab === "orphans" && (
            <>
              <p className="mt-4 text-[13px] text-fg-tertiary">Pages with no links in or out. Link them from a structure page, or let the librarian propose links.</p>
              <ul className="mt-4 divide-y divide-border-row">
                {orphans.isPending && <Skeleton className="h-12 w-full" />}
                {orphans.data?.rows.map((r) => (
                  <li key={r.id}>
                    <Link href={pageHref(r.id, r.title)} className="flex items-center gap-3 rounded-md px-2 py-3 hover:bg-hover">
                      <Icon name="artifacts" size={14} className="text-fg-tertiary" />
                      <span className="flex-1 text-sm text-fg-2">{r.title}</span>
                      <span className="text-xs text-fg-tertiary">{r.path.split("/").slice(0, -1).join("/") || "Top level"}</span>
                    </Link>
                  </li>
                ))}
                {orphans.data && !orphans.data.rows.length && <li className="py-8 text-center text-[13px] text-fg-tertiary">No orphans: every page links somewhere.</li>}
              </ul>
            </>
          )}
        </div>
      </div>
      <StatusBar left="Private pages are only checked against your own pages" right={`${unresolved.length} unresolved · ${orphans.data?.rows.length ?? 0} orphans`} />
    </>
  );
}

/** The weekly librarian pass: a scheduled task whose suggestions land in Needs you. */
function Librarian() {
  const tasks = useScheduledTasks().data?.tasks ?? [];
  const task = tasks.find((t) => t.name === LIBRARIAN);
  const save = useSaveScheduledTask();
  const run = useRunScheduledTask();
  const setUp = () =>
    save.mutate({
      body: { name: LIBRARIAN, kind: "prompt", instructions: LIBRARIAN_PROMPT, frequency: "weekly", weekdays: [1], at_time: "09:00", timezone: Intl.DateTimeFormat().resolvedOptions().timeZone, enabled: true },
    });
  return (
    <div className="mt-6 flex items-center gap-4 rounded-xl border border-border bg-surface p-4">
      <span className="grid size-10 flex-none place-items-center rounded-full bg-[#3FC1B033] text-[#3FC1B0]">
        <Icon name="bulb" size={18} />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-fg">
          Weekly librarian pass{task && <span className="font-normal text-fg-tertiary"> · Mondays {task.at_time}</span>}
        </p>
        <p className="text-[13px] text-fg-secondary">
          {task ? "A persona proposes links for unlinked mentions and lists orphans. Its suggestions wait for you in Needs you." : "Have a persona propose the links you missed once a week. Nothing changes until you accept it."}
        </p>
        {(save.error || run.error) && <p className="mt-1 text-xs text-state-blocked">{(save.error ?? run.error)?.message}</p>}
      </div>
      {task ? (
        <>
          <Button onClick={() => run.mutate(task.id)} disabled={run.isPending}>
            {run.isPending ? "Starting…" : "Run now"}
          </Button>
          <Link href="/scheduled" className="text-xs text-fg-tertiary hover:text-fg-secondary">
            Schedule
          </Link>
        </>
      ) : (
        <Button variant="primary" onClick={setUp} disabled={save.isPending}>
          Set it up
        </Button>
      )}
    </div>
  );
}
