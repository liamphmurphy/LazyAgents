"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import type { Ticket } from "@/lib/api/client";
import { useActivity, useBootstrap, useTicket, useTickets, useUpdateTicket, hasLiveRun } from "@/lib/api/hooks";
import { Button, ButtonLink, IconButton } from "@/components/ui/button";
import { Banner, EmptyState, Skeleton, TextArea } from "@/components/ui/primitives";
import { Markdown } from "@/components/domain/markdown";
import { TopBar } from "@/components/shell/topbar";
import { BacklinksPanel } from "@/components/domain/backlinks";
import { ContextArtifacts } from "./context-artifacts";
import { Criteria } from "./criteria";
import { CodeSection, Properties } from "./properties";
import { ApprovalBanner, RunCard } from "./run-parts";
import { Composer, Timeline } from "./timeline";

function TitleEditor({ t }: { t: Ticket }) {
  const update = useUpdateTicket(t.key);
  const [editing, setEditing] = useState(false);
  if (!editing) {
    return (
      <h1 className="type-display">
        <button type="button" onClick={() => setEditing(true)} className="-mx-1 rounded-md px-1 text-left hover:bg-hover" title="Edit title">
          {t.title}
        </button>
      </h1>
    );
  }
  const commit = (v: string) => {
    setEditing(false);
    if (v.trim() && v.trim() !== t.title) update.mutate({ title: v.trim() });
  };
  return (
    <>
      <label htmlFor="title" className="sr-only">
        Title
      </label>
      <input
        id="title"
        autoFocus
        defaultValue={t.title}
        maxLength={300}
        onBlur={(e) => commit(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") commit(e.currentTarget.value);
          if (e.key === "Escape") setEditing(false);
        }}
        className="type-display -mx-1 w-full rounded-md border border-accent bg-surface px-1 text-fg outline-none"
      />
    </>
  );
}

function Description({ t }: { t: Ticket }) {
  const update = useUpdateTicket(t.key);
  const [draft, setDraft] = useState<string | null>(null);
  if (draft === null) {
    return t.description_md ? (
      <div className="group relative">
        <Markdown className="type-reading text-fg-2">{t.description_md}</Markdown>
        <Button variant="ghost" icon="edit" onClick={() => setDraft(t.description_md)} className="mt-1 h-7 px-2 text-xs text-fg-tertiary">
          Edit description
        </Button>
      </div>
    ) : (
      <button type="button" onClick={() => setDraft("")} className="w-full rounded-md py-1 text-left type-reading text-fg-tertiary hover:text-fg-secondary">
        Add a description…
      </button>
    );
  }
  const save = () => {
    if (draft !== t.description_md) update.mutate({ description_md: draft });
    setDraft(null);
  };
  return (
    <div className="flex flex-col gap-2">
      <label htmlFor="description" className="sr-only">
        Description (markdown)
      </label>
      <TextArea
        id="description"
        autoFocus
        rows={6}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) save();
          if (e.key === "Escape") setDraft(null);
        }}
        placeholder="What needs to happen and why. Markdown works."
      />
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={() => setDraft(null)}>
          Cancel
        </Button>
        <Button variant="primary" onClick={save} hint="⌘↵">
          Save
        </Button>
      </div>
    </div>
  );
}

/** Position within the project's list, for the "3 / 15" pager. */
function usePager(t: Ticket | undefined) {
  const list = useTickets({ project: t?.project.key, view: "all" });
  const keys = list.data?.map((x) => x.key) ?? [];
  const i = t ? keys.indexOf(t.key) : -1;
  return { index: i, total: keys.length, prev: i > 0 ? keys[i - 1] : undefined, next: i >= 0 && i < keys.length - 1 ? keys[i + 1] : undefined };
}

function DetailSkeleton() {
  return (
    <div aria-busy="true" className="mx-auto flex w-full max-w-[var(--reading-max)] flex-col gap-4 px-7 py-10">
      <Skeleton className="h-7 w-2/3" />
      <Skeleton className="h-3 w-full" />
      <Skeleton className="h-3 w-5/6" />
      <Skeleton className="mt-6 h-3 w-40" />
      {[0, 1, 2].map((i) => (
        <Skeleton key={i} className="h-3 w-1/2" />
      ))}
    </div>
  );
}

export function TicketScreen({ ticketKey }: { ticketKey: string }) {
  const router = useRouter();
  const ticket = useTicket(ticketKey);
  const activity = useActivity(ticketKey, hasLiveRun(ticket.data));
  const boot = useBootstrap();
  const t = ticket.data;
  const pager = usePager(t);
  const [copied, setCopied] = useState(false);

  // J/K step through the project's tickets.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const el = e.target as HTMLElement;
      if (e.metaKey || e.ctrlKey || e.altKey || ["INPUT", "TEXTAREA"].includes(el.tagName) || el.isContentEditable) return;
      if (e.key === "j" && pager.next) router.push(`/tickets/${pager.next}`);
      if (e.key === "k" && pager.prev) router.push(`/tickets/${pager.prev}`);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [pager.next, pager.prev, router]);

  if (ticket.isError) {
    const missing = "status" in ticket.error && ticket.error.status === 404;
    return (
      <>
        <TopBar crumbs={[{ label: "Tickets", href: "/tickets" }, { label: ticketKey, mono: true }]} />
        {missing ? (
          <EmptyState icon="tickets" title={`${ticketKey} doesn't exist`} actions={<ButtonLink href="/tickets" variant="outline">All tickets</ButtonLink>}>
            It may have been moved, or the link has a typo.
          </EmptyState>
        ) : (
          <div className="p-6">
            <Banner tone="blocked" title="Couldn't load this ticket">
              {ticket.error.message}
            </Banner>
          </div>
        )}
      </>
    );
  }

  return (
    <>
      <TopBar
        crumbs={
          t
            ? [
                { label: t.project.name, swatch: t.project.color, href: `/tickets?project=${t.project.key}` },
                ...(t.epic ? [{ label: t.epic.name }] : []),
                { label: t.key, mono: true },
              ]
            : [{ label: "Tickets", href: "/tickets" }, { label: ticketKey, mono: true }]
        }
        actions={
          <>
            {pager.total > 0 && pager.index >= 0 && (
              <span className="font-mono text-xs text-fg-tertiary" aria-label={`Ticket ${pager.index + 1} of ${pager.total}`}>
                {pager.index + 1} / {pager.total}
              </span>
            )}
            <IconButton icon="chev" label="Previous ticket (K)" bordered disabled={!pager.prev} onClick={() => pager.prev && router.push(`/tickets/${pager.prev}`)} className="rotate-180" />
            <IconButton icon="chev" label="Next ticket (J)" bordered disabled={!pager.next} onClick={() => pager.next && router.push(`/tickets/${pager.next}`)} />
            <IconButton
              icon={copied ? "check" : "link"}
              label={copied ? "Link copied" : "Copy link"}
              onClick={() => {
                void navigator.clipboard.writeText(window.location.href);
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
              }}
            />
          </>
        }
      />
      {!t || !boot.data ? (
        <DetailSkeleton />
      ) : (
        <div className="flex flex-1 flex-wrap">
          <article className="flex min-w-0 flex-[999_1_480px] justify-center px-7 py-8 max-sm:px-4">
            <div className="flex w-full max-w-[700px] flex-col gap-7">
              {t.approvals.map((ap) => (
                <ApprovalBanner key={ap.id} t={t} ap={ap} />
              ))}
              <div className="flex flex-col gap-3">
                <TitleEditor t={t} />
                <Description t={t} />
              </div>
              <Criteria t={t} />
              <ContextArtifacts t={t} />
              <section className="flex flex-col gap-4">
                <h2 className="type-heading">Activity</h2>
                {activity.data ? (
                  <Timeline events={activity.data} ticketKey={t.key} boot={boot.data} run={t.run} />
                ) : (
                  <Skeleton className="h-3 w-1/2" />
                )}
                <Composer ticketKey={t.key} />
              </section>
            </div>
          </article>
          <aside aria-label="Properties" className="flex flex-[1_1_300px] flex-col gap-6 border-l border-border-subtle bg-panel px-4 py-6 lg:max-w-[340px]">
            <Properties t={t} boot={boot.data} />
            {t.run && <RunCard t={t} run={t.run} />}
            <CodeSection t={t} />
            <section className="flex flex-col gap-2 px-1">
              <h2 className="text-[12.5px] text-fg-tertiary">Backlinks</h2>
              <BacklinksPanel kind="ticket" id={t.id} compact />
            </section>
          </aside>
        </div>
      )}
    </>
  );
}
