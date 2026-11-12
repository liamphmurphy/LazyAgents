"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { useCreateDoc, useDocs } from "@/lib/api/docs-hooks";
import { ago } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** The Docs app: every document the person has, from here or from any chat, and a way to start one. */
export function DocsScreen() {
  const docs = useDocs();
  const create = useCreateDoc();
  const router = useRouter();
  const [title, setTitle] = useState("");
  const start = (e?: FormEvent) => {
    e?.preventDefault();
    if (create.isPending) return;
    create.mutate(title, { onSuccess: (d) => router.push(`/apps/docs/${d.id}`) });
  };
  return (
    <div className="flex h-app flex-col">
      <TopBar crumbs={[{ label: "Apps" }, { label: "Docs" }]} />
      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-[860px] flex-col gap-5 px-6 py-6">
          <form onSubmit={start} className="flex gap-2 rounded-xl border border-border-strong bg-surface p-1 pl-3 focus-within:border-accent">
            <input aria-label="New document title" className="flex-1 bg-transparent text-[13.5px] text-fg outline-none placeholder:text-fg-tertiary" placeholder="Start a document: give it a title, or leave it blank" maxLength={200} value={title} onChange={(e) => setTitle(e.target.value)} />
            <Button type="submit" variant="primary" icon="plus" disabled={create.isPending}>
              {create.isPending ? "Creating…" : "New document"}
            </Button>
          </form>
          {create.isError && <p role="alert" className="text-[12px] text-pill-blocked-fg">{errorText(create.error)}</p>}
          {docs.isError ? (
            <Banner tone="blocked" title="Couldn’t load your documents">{errorText(docs.error)}</Banner>
          ) : !docs.data ? (
            <div aria-busy="true" className="flex flex-col gap-2">
              {[0, 1, 2].map((i) => <Skeleton key={i} className="h-[52px]" />)}
            </div>
          ) : docs.data.length === 0 ? (
            <p className="py-10 text-center text-[13px] text-fg-tertiary">No documents yet. Start one above, or ask for one in any chat: it shows up here too.</p>
          ) : (
            <ul aria-label="Documents" className="flex flex-col">
              {docs.data.map((d) => (
                <li key={d.id}>
                  <Link href={`/apps/docs/${d.id}`} className="flex items-center gap-3 rounded-lg px-3 py-2.5 hover:bg-hover">
                    <Icon name="edit" size={15} className="flex-none text-fg-tertiary" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[13.5px] font-medium text-fg">{d.title}</span>
                      {d.chat_title !== d.title && <span className="block truncate text-[12px] text-fg-tertiary">From {d.chat_title || "a chat"}</span>}
                    </span>
                    <span className="flex-none text-[12px] text-fg-tertiary">{ago(d.updated_at)}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      </main>
    </div>
  );
}
