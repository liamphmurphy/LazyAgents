"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, type ReactNode } from "react";
import { useBootstrap, useMemories } from "@/lib/api/hooks";
import { usePersonas, type Persona } from "@/lib/api/persona-hooks";
import { MAX_PERSONAS, sortPersonas } from "@/lib/persona-ui";
import { cx } from "@/lib/cx";
import { HumanAvatar } from "@/components/domain/avatars";
import { Content, TopBar } from "@/components/shell/topbar";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { PersonaAvatar } from "../chat/_components/persona";
import { PersonaEditor } from "../chat/_components/persona-editor";
import { ExportMenu, ImportView, MemorySection } from "./memory-section";
import { DefaultBadge, DeleteDialog, PersonaProfile } from "./persona-profile";
import { ProfileSection } from "./profile-section";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

const railItem = "flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-left text-[13px]";

function RailButton({ on, onClick, lead, testId, children }: { on: boolean; onClick: () => void; lead: ReactNode; testId?: string; children: ReactNode }) {
  return (
    <button type="button" aria-pressed={on} data-testid={testId} onClick={onClick} className={cx(railItem, on ? "bg-selected font-medium text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg")}>
      {lead}
      {children}
    </button>
  );
}

function PlainHeader({ lead, title, detail, actions }: { lead: ReactNode; title: string; detail: string; actions?: ReactNode }) {
  return (
    <header className="flex items-center gap-4">
      {lead}
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <h2 className="truncate text-lg font-semibold text-fg">{title}</h2>
        <p className="text-xs text-fg-secondary">{detail}</p>
      </div>
      {actions}
    </header>
  );
}

/** Personas and everything remembered, on one page: You or a persona on the left, its profile and memories on the right. */
export function CustomizationScreen() {
  const router = useRouter();
  const search = useSearchParams();
  const focus = search.get("id") ?? undefined;
  const who = search.get("who") ?? "";
  const boot = useBootstrap();
  const list = usePersonas();
  const count = useMemories();
  const all = list.data?.personas ?? [];
  const personas = sortPersonas(all);
  const selected = personas.find((p) => p.id === who);
  // A persona that's gone falls back to everything; while loading, keep what the URL says.
  const whose = who === "person" || !list.data ? who : (selected?.id ?? "");
  const scoped = useMemories("", undefined, 0, whose);
  const [editing, setEditing] = useState<Persona | "new" | null>(null);
  const [deleting, setDeleting] = useState<Persona | null>(null);
  const [importing, setImporting] = useState(false);
  const [editingProfile, setEditingProfile] = useState(false);
  const total = count.data?.total;
  const pick = (v: string) => {
    setImporting(false);
    setEditingProfile(false);
    router.replace(v ? `/customization?who=${encodeURIComponent(v)}` : "/customization", { scroll: false });
  };
  const me = boot.data?.me;
  return (
    <section aria-label="Customization">
      <TopBar
        crumbs={[{ label: "Customization" }]}
        actions={
          <>
            {total !== undefined && <span className="text-xs text-fg-tertiary">{total} entries</span>}
            <button
              type="button"
              aria-pressed={importing}
              onClick={() => setImporting((v) => !v)}
              className={cx(
                "inline-flex h-[30px] items-center gap-1.5 rounded-md border px-3 text-[12.5px] text-fg",
                importing ? "border-accent bg-selected" : "border-btn-secondary-border bg-btn-secondary hover:bg-hover",
              )}
            >
              <Icon name="upload" size={14} />
              Import
            </button>
            <ExportMenu />
          </>
        }
      />
      <Content>
        <div className="flex gap-8 max-md:flex-col max-md:gap-4">
          <nav aria-label="Customize" className="flex w-52 flex-none flex-col gap-px max-md:w-full">
            <RailButton on={!whose} onClick={() => pick("")} lead={<Icon name="bookmark" size={16} />}>
              Everything
            </RailButton>
            <RailButton on={whose === "person"} onClick={() => pick("person")} lead={<HumanAvatar name={me?.name ?? "You"} initials={me?.initials} src={me?.avatar_url} size={20} label={false} />}>
              You
            </RailButton>
            <span className="px-2 pb-1 pt-3 text-xs font-medium text-fg-tertiary">Personas</span>
            {list.isError ? (
              <Banner tone="blocked" title="Couldn’t load personas" actions={<Button variant="outline" onClick={() => list.refetch()}>Retry</Button>}>
                {errorText(list.error)}
              </Banner>
            ) : !list.data ? (
              <div aria-busy="true" className="flex flex-col gap-1">
                <Skeleton className="h-8" />
                <Skeleton className="h-8" />
              </div>
            ) : (
              <>
                {personas.map((p) => (
                  <RailButton key={p.id} testId="persona-row" on={whose === p.id} onClick={() => pick(p.id)} lead={<PersonaAvatar persona={p} size={20} />}>
                    <span className="min-w-0 flex-1 truncate">{p.name}</span>
                    {p.is_default && <DefaultBadge small />}
                  </RailButton>
                ))}
                {personas.length === 0 && <p className="px-2 py-1 text-xs text-fg-secondary">No personas yet. The built-in Assistant is your default.</p>}
                {all.length < MAX_PERSONAS && (
                  <button type="button" onClick={() => setEditing("new")} className={cx(railItem, "text-accent hover:bg-hover")}>
                    <Icon name="plus" size={16} />
                    New persona
                  </button>
                )}
              </>
            )}
          </nav>
          <div className="flex min-w-0 flex-1 flex-col gap-5">
            {importing ? (
              <div className="mx-auto w-full max-w-2xl">
                <ImportView onClose={() => setImporting(false)} />
              </div>
            ) : (
              <>
                {selected ? (
                  <PersonaProfile p={selected} memories={scoped.data?.total} onEdit={() => setEditing(selected)} onDelete={() => setDeleting(selected)} />
                ) : whose === "person" ? (
                  <>
                    <PlainHeader
                      lead={<HumanAvatar name={me?.name ?? "You"} initials={me?.initials} color={me?.avatar_color} src={me?.avatar_url} size={72} label={false} />}
                      title={me?.name ?? "You"}
                      detail={[me?.role_title, "What your personas remember about you."].filter(Boolean).join(" · ")}
                      actions={<Button variant="secondary" aria-pressed={editingProfile} onClick={() => setEditingProfile((v) => !v)}>{editingProfile ? "Done" : "Edit profile"}</Button>}
                    />
                    {editingProfile && (
                      <section aria-label="Profile">
                        <ProfileSection />
                      </section>
                    )}
                  </>
                ) : (
                  <PlainHeader lead={<span className="grid size-[72px] place-items-center rounded-full bg-selected text-fg-secondary"><Icon name="bookmark" size={28} /></span>} title="Everything" detail="Every memory: about you, and each persona’s own." />
                )}
                <div className="border-t border-border-subtle pt-4">
                  <MemorySection key={whose} whose={whose} focus={focus} heading={selected ? `${selected.name}’s memory` : whose === "person" ? "About you" : "All memories"} />
                </div>
              </>
            )}
          </div>
        </div>
      </Content>
      {editing && <PersonaEditor key={editing === "new" ? "new" : editing.id} persona={editing} onClose={() => setEditing(null)} />}
      {deleting && <DeleteDialog persona={deleting} all={all} onClose={() => setDeleting(null)} />}
    </section>
  );
}
