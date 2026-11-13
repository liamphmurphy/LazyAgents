"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ago, pageHref } from "@/lib/knowledge";
import { useCreatePage, usePages } from "@/lib/api/knowledge-hooks";
import { useBootstrap } from "@/lib/api/hooks";
import { Icon, type IconName } from "@/components/ui/icon";
import { Kbd, Skeleton } from "@/components/ui/primitives";
import { AUDIENCE, StatusBar, useToday } from "./kit";
import { useKnowledge, useKnowledgeTab } from "./shell";

const HOME_TAB = { id: "home", title: "New tab", kind: "home" as const };

function greeting(d: Date | null) {
  if (!d) return "Welcome back";
  const h = d.getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

/** A new tab: quick ways in, what you touched last, the inbox, and the vault import. */
export function KnowledgeHome() {
  useKnowledgeTab(HOME_TAB);
  const k = useKnowledge();
  const today = useToday();
  const router = useRouter();
  const me = useBootstrap().data?.me;
  const recent = usePages({ limit: 8, include_agent: k.view === "agent", audience: k.view === "agent" ? "agent" : undefined });
  const inbox = usePages({ folder: "Inbox", limit: 3 });
  const create = useCreatePage();
  const newPage = () => create.mutate({ title: "Untitled", body_md: "" }, { onSuccess: (p) => router.push(pageHref(p.id, p.title)) });
  const actions: [IconName, string, string, string, () => void][] = [
    ["clock", "Today's note", "⌘D", "text-state-progress", k.openToday],
    ["edit", "New page", "", "text-accent", newPage],
    ["inbox", "Quick capture", "⌥⌘N", "text-[#3FC1B0]", k.openCapture],
    ["search", "Go to page", "⌘O", "text-state-review", k.openSwitcher],
  ];
  const name = me?.name?.split(/\s+/)[0];

  return (
    <>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[760px] px-6 pb-12 pt-14">
          <h1 className="text-[28px] font-bold tracking-[-0.02em] text-fg">
            {greeting(today)}
            {/* Only once on the client: the name comes from data the server render doesn't have. */}
            {today && name ? `, ${name}` : ""}
          </h1>
          <p className="mt-1 text-sm text-fg-tertiary">
            {today?.toLocaleDateString("en", { weekday: "long", day: "numeric", month: "long" })}
            {inbox.data && inbox.data.rows.length > 0 && ` · ${inbox.data.has_more ? "3+" : inbox.data.rows.length} in your inbox`}
          </p>
          <div className="mt-8 grid grid-cols-2 gap-2.5 sm:grid-cols-4">
            {actions.map(([icon, label, key, cls, fn]) => (
              <button key={label} onClick={fn} className="flex h-[92px] flex-col justify-between rounded-xl border border-border bg-surface p-4 text-left hover:border-border-strong hover:bg-hover">
                <span className="flex items-start justify-between">
                  <Icon name={icon} size={19} className={cls} />
                  {key && <Kbd>{key}</Kbd>}
                </span>
                <span className="text-sm font-semibold text-fg">{label}</span>
              </button>
            ))}
          </div>

          <div className="mt-9 flex items-baseline justify-between">
            <h2 className="text-[10px] font-bold uppercase tracking-[0.06em] text-fg-tertiary">Recent</h2>
            <button onClick={k.openSwitcher} className="text-xs font-medium text-accent hover:underline">
              Find a page
            </button>
          </div>
          <ul className="mt-2">
            {recent.isPending &&
              Array.from({ length: 5 }, (_, i) => (
                <li key={i} className="py-3">
                  <Skeleton className="h-4 w-2/3" />
                </li>
              ))}
            {recent.data?.rows.map((r) => (
              <li key={r.id} className="border-b border-border-row">
                <Link href={pageHref(r.id, r.title)} className="grid h-[46px] grid-cols-[20px_1fr_1fr_90px_80px] items-center gap-3 rounded-md px-2 hover:bg-hover">
                  <Icon name={r.properties?.query ? "board" : "artifacts"} size={14} className="text-fg-tertiary" />
                  <span className="truncate text-sm font-medium text-fg">{r.title}</span>
                  <span className="truncate text-[13px] text-fg-tertiary">{r.path.split("/").slice(0, -1).join(" / ") || "Top level"}</span>
                  <span className="flex items-center gap-1.5 text-xs text-fg-secondary">
                    <Icon name={AUDIENCE[r.audience].icon} size={12} />
                    {AUDIENCE[r.audience].label}
                  </span>
                  <span className="text-right text-xs text-fg-tertiary">{ago(r.updated_at)}</span>
                </Link>
              </li>
            ))}
            {recent.data && recent.data.rows.length === 0 && <li className="py-6 text-center text-[13px] text-fg-tertiary">No pages yet.</li>}
          </ul>

          <div className="mt-6 grid gap-3 sm:grid-cols-2">
            <div className="rounded-xl border border-border bg-surface p-4">
              <p className="flex items-center gap-2 text-sm font-semibold text-fg">
                <Icon name="inbox" size={15} className="text-[#3FC1B0]" /> Inbox
              </p>
              <ul className="mt-2 space-y-1 text-[13px] text-fg-secondary">
                {inbox.data?.rows.map((r) => (
                  <li key={r.id}>
                    <Link href={pageHref(r.id, r.title)} className="hover:text-fg hover:underline">
                      {r.title}
                    </Link>
                  </li>
                ))}
                {inbox.data && !inbox.data.rows.length && <li>Nothing to file. Capture with ⌥⌘N.</li>}
              </ul>
            </div>
            <div className="rounded-xl border border-border bg-surface p-4">
              <p className="flex items-center gap-2 text-sm font-semibold text-fg">
                <Icon name="upload" size={15} className="text-accent" /> Bring your Obsidian vault
              </p>
              <p className="mt-2 text-[13px] text-fg-secondary">Upload a zip. You see a dry run before anything is written, and can undo it.</p>
              <button onClick={k.openImport} className="mt-2 text-xs font-medium text-accent hover:underline">
                Import vault →
              </button>
            </div>
          </div>
        </div>
      </div>
      <StatusBar left={k.view === "agent" ? "Agent knowledge · written by runs" : "Your knowledge · private by default"} right="Online over Tailscale" />
    </>
  );
}
