"use client";

import Link from "next/link";
import type { AgentSpec } from "@/lib/api/client";
import { useWikiTree } from "@/lib/api/hooks";
import { ago } from "@/lib/format";
import { wikiHref } from "@/lib/wikilinks";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Card, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Flag } from "@/components/domain/labels";

const INJECT: Record<NonNullable<AgentSpec["memory"]>["inject"], string> = {
  selective: "selective previews",
  index: "the Memory page",
  linked: "the Memory page and pages linked to the ticket",
  search: "linked pages and the best wiki matches",
};

/** Pages in the agent's wiki namespace, agents/<handle>/. */
export function MemoryTab({ handle, memory }: { handle: string; memory: AgentSpec["memory"] }) {
  const tree = useWikiTree();
  const prefix = `agents/${handle}/`;
  const index = `${prefix}Memory`;
  const pages = (tree.data?.pages ?? [])
    .filter((p) => p.path.startsWith(prefix))
    .sort((a, b) => Number(b.path === index) - Number(a.path === index) || b.updated_at.localeCompare(a.updated_at));
  const m = memory ?? { enabled: true, inject: "selective" as const, max_tokens: 1500 };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <span className="font-mono text-13">{prefix}</span>
        {tree.data && (
          <span className="text-12 text-fg-tertiary">
            {pages.length} page{pages.length === 1 ? "" : "s"}
          </span>
        )}
        <Link href={wikiHref(index)} className="ml-auto text-12 text-accent hover:underline">
          Open in wiki →
        </Link>
      </div>
      <p className="text-12 text-fg-secondary">
        {m.enabled ? (
          <>
            Each run starts with <b className="font-medium">{INJECT[m.inject]}</b>, up to <b className="font-medium">{m.max_tokens.toLocaleString()} tokens</b>.{" "}
          </>
        ) : (
          "Runs don't start with wiki context. "
        )}
        <Link href={`/agents/${handle}?tab=settings`} className="text-accent hover:underline">
          Change in Settings
        </Link>
      </p>
      {tree.isError ? (
        <Banner tone="blocked" title="Couldn’t load the wiki" actions={<Button onClick={() => tree.refetch()}>Retry</Button>}>
          {tree.error.message}
        </Banner>
      ) : !tree.data ? (
        <Skeleton className="h-48" />
      ) : pages.length === 0 ? (
        <EmptyState icon="wiki" title="No memory pages yet">
          Pages the agent writes under <code className="inline">{prefix}</code> appear here.
        </EmptyState>
      ) : (
        <Card bodyClassName="p-0">
          <ul aria-label="Memory pages">
            {pages.map((p, i) => (
              <li key={p.id} className={i ? "border-t border-border-row" : undefined}>
                <Link href={wikiHref(p.path)} className="flex items-center gap-3 px-[18px] py-2.5 hover:bg-row-hover">
                  <Icon name="wiki" size={14} className="flex-none text-fg-tertiary" />
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="flex items-center gap-2 text-13 font-medium">
                      <span className="truncate">{p.title}</span>
                      {p.path === index && <Flag tone="neutral">index</Flag>}
                      {!p.published && <Flag tone="human">suggestion only</Flag>}
                      {p.suggestions > 0 && (
                        <Flag tone="review">
                          {p.suggestions} suggestion{p.suggestions === 1 ? "" : "s"}
                        </Flag>
                      )}
                    </span>
                    <span className="truncate font-mono text-11 text-fg-tertiary">{p.path}</span>
                  </span>
                  <span className="flex-none text-12 text-fg-tertiary">
                    {p.author.name} · {ago(p.updated_at)}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      )}
      <p className="text-12 text-fg-tertiary">
        The agent can always write here. Pages elsewhere follow <code className="inline">wiki:write</code>: with Approval they arrive as suggestions.
      </p>
    </div>
  );
}
