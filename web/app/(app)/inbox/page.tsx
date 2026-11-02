"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import {
  useBootstrap,
  useInbox,
  useReadInbox,
  useTicket,
  useDecideApproval,
} from "@/lib/api/hooks";
import type { Schemas } from "@/lib/api/client";
import { Button, ButtonLink } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/primitives";
import {
  ApprovalBanner,
  DispatchBanner,
  RunCard,
} from "../tickets/[key]/run-parts";
import { cx } from "@/lib/cx";
import { ago } from "@/lib/format";

type Item = Schemas["InboxItem"];
const tabs = [
  "all",
  "approval",
  "dispatch",
  "mention",
  "review",
  "blocked",
  "suggestion",
  "planning",
] as const;
const names: Record<string, string> = {
  all: "All",
  approval: "Approvals",
  dispatch: "Dispatch",
  mention: "Mentions",
  review: "Reviews",
  blocked: "Blocked",
  suggestion: "Wiki edits",
  planning: "Plans",
};

function Detail({ item }: { item: Item }) {
  const ticket = useTicket(item.ticket_key);
  const ap = ticket.data?.approvals.find((a) => `approval:${a.id}` === item.id);
  return (
    <aside className="min-w-0 flex-[1_1_380px] border-l border-border bg-panel p-5">
      <span className="type-micro uppercase text-fg-tertiary">
        {names[item.kind]} · {item.project || "Workspace"}
      </span>
      <h2 className="type-heading mt-3">{item.title}</h2>
      <p className="my-4 whitespace-pre-wrap break-words text-sm text-fg-secondary">
        {item.summary}
      </p>
      {ticket.isLoading && item.ticket_key && <Skeleton className="h-24" />}
      {ticket.isError && item.ticket_key && (
        <p role="alert" className="text-sm text-pill-blocked-fg">
          Couldn’t load this ticket.
        </p>
      )}
      {ticket.data && ap && (
        <ApprovalBanner key={ap.id} t={ticket.data} ap={ap} expanded />
      )}
      {ticket.data && item.kind === "dispatch" && (
        <DispatchBanner t={ticket.data} />
      )}
      {ticket.data?.run && (
        <div className="my-5">
          <RunCard t={ticket.data} run={ticket.data.run} />
        </div>
      )}
      {item.resolved && (
        <p className="my-4 text-sm text-fg-tertiary">
          This item has been resolved.
        </p>
      )}
      <ButtonLink href={item.href} variant="outline">
        {item.kind === "planning" ? "Review plan" : item.kind === "suggestion"
          ? "Review wiki edit"
          : item.kind === "blocked"
            ? "Open run"
            : "Open ticket"}
      </ButtonLink>
    </aside>
  );
}
export default function Page() {
  const [project, setProject] = useState("");
  const [tab, setTab] = useState<string>("all");
  const [selected, setSelected] = useState<string>();
  const { data: bootstrap } = useBootstrap();
  const inbox = useInbox(project || undefined);
  const read = useReadInbox();
  const items = (inbox.data ?? []).filter(
    (i) => tab === "all" || i.kind === tab,
  );
  const item = items.find((i) => i.id === selected) ?? items[0];
  const decide = useDecideApproval(item?.ticket_key ?? "");
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (
        e.ctrlKey ||
        e.metaKey ||
        e.altKey ||
        (e.target instanceof HTMLElement &&
          (e.target.isContentEditable ||
            ["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes(
              e.target.tagName,
            ))) ||
        document.querySelector('[role="dialog"]')
      )
        return;
      const k = e.key.toLowerCase();
      if (k === "j" || k === "k") {
        e.preventDefault();
        const pos = items.findIndex((i) => i.id === item?.id);
        setSelected(
          items[(pos + (k === "j" ? 1 : -1) + items.length) % items.length]?.id,
        );
      }
      if (
        (k === "a" || k === "d") &&
        item?.kind === "approval" &&
        !item.resolved &&
        !decide.isPending
      ) {
        e.preventDefault();
        decide.mutate({
          id: item.id.slice(9),
          decision: k === "a" ? "approve" : "deny",
        });
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [items, item, decide]);
  const choose = (i: Item) => {
    setSelected(i.id);
    if (!i.read) read.mutate(i.id);
  };
  return (
    <>
      <header className="flex h-[54px] items-center border-b border-border px-6 type-heading">
        Inbox
      </header>
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-5 py-3">
        {tabs.map((t) => (
          <button
            key={t}
            onClick={() => {
              setTab(t);
              setSelected(undefined);
            }}
            aria-pressed={tab === t}
            className={cx(
              "rounded-md px-2.5 py-1 text-sm",
              tab === t
                ? "bg-selected text-fg"
                : "text-fg-tertiary hover:bg-hover",
            )}
          >
            {names[t]}{" "}
            <span className="font-mono text-xs">
              {
                (inbox.data ?? []).filter(
                  (i) =>
                    (t === "all" || i.kind === t) && !i.read && !i.resolved,
                ).length
              }
            </span>
          </button>
        ))}
        <label className="ml-auto text-xs text-fg-tertiary">
          Project{" "}
          <select
            aria-label="Filter project"
            value={project}
            onChange={(e) => setProject(e.target.value)}
            className="rounded-md border border-border bg-surface px-2 py-1 text-fg"
          >
            <option value="">All projects</option>
            {bootstrap?.projects.map((p) => (
              <option key={p.key}>{p.key}</option>
            ))}
          </select>
        </label>
      </div>
      {inbox.isLoading ? (
        <div aria-busy="true" className="space-y-3 p-5">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-[60px]" />
          ))}
        </div>
      ) : inbox.isError ? (
        <div role="alert" className="p-6 text-pill-blocked-fg">
          Couldn’t load your inbox.{" "}
          <Button onClick={() => inbox.refetch()}>Retry</Button>
        </div>
      ) : items.length === 0 ? (
        <div className="m-auto p-10 text-center">
          <h2 className="type-heading">You’re all caught up</h2>
          <p className="mt-2 text-sm text-fg-tertiary">
            Approvals, dispatch requests, mentions and reviews will appear here.
          </p>
          <Link
            href="/tickets"
            className="mt-4 inline-block text-sm text-accent"
          >
            View tickets
          </Link>
        </div>
      ) : (
        <div className="flex flex-1 flex-wrap">
          <section
            aria-label="Inbox items"
            className="min-w-0 flex-[2_1_480px]"
          >
            {[
              [
                "Needs you",
                items.filter(
                  (i) => !i.resolved && !i.read && i.kind !== "review",
                ),
              ],
              [
                "Waiting on others",
                items.filter(
                  (i) => !i.resolved && !i.read && i.kind === "review",
                ),
              ],
              ["Earlier", items.filter((i) => i.resolved || i.read)],
            ].map(([name, group]) => (
              <div key={name as string}>
                {(group as Item[]).length > 0 && (
                  <h2 className="flex h-[34px] items-center bg-surface-sunken px-5 text-xs text-fg-tertiary">
                    {name as string}
                  </h2>
                )}
                {(group as Item[]).map((i) => (
                  <button
                    key={i.id}
                    onClick={() => choose(i)}
                    aria-pressed={item?.id === i.id}
                    className={cx(
                      "flex min-h-[60px] w-full items-center gap-3 border-b border-border-subtle px-5 py-3 text-left",
                      item?.id === i.id ? "bg-selected" : "hover:bg-row-hover",
                    )}
                  >
                    <span
                      aria-label={i.read ? "Read" : "Unread"}
                      className={cx(
                        "size-[7px] shrink-0 rounded-full",
                        !i.read && !i.resolved ? "bg-accent" : "bg-track",
                      )}
                    />
                    <span className="min-w-0 flex-1">
                      <span
                        className={cx(
                          "block truncate text-sm",
                          !i.read ? "font-semibold" : "font-medium",
                        )}
                      >
                        <span className="mr-2 text-xs text-pill-human-fg">
                          {names[i.kind]}
                        </span>
                        <span className="mr-2 font-mono text-xs text-fg-tertiary">
                          {i.ticket_key}
                        </span>
                        {i.title}
                      </span>
                      <span className="mt-1 block truncate text-xs text-fg-tertiary">
                        {i.summary}
                      </span>
                    </span>
                    <span className="shrink-0 text-xs text-fg-tertiary">
                      {ago(i.created_at)}
                    </span>
                  </button>
                ))}
              </div>
            ))}
          </section>
          {item && <Detail key={item.id} item={item} />}
        </div>
      )}
      {(read.isError || decide.isError) && (
        <p role="alert" className="p-3 text-sm text-pill-blocked-fg">
          {read.error?.message ?? decide.error?.message}
        </p>
      )}
    </>
  );
}
