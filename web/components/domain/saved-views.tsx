"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Dialog, DropdownMenu } from "radix-ui";
import { useState, type FormEvent, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { ApiError } from "@/lib/api/client";
import { useCreateSavedView, useDeleteSavedView, useSavedViews, useUpdateSavedView, type SavedView } from "@/lib/api/hooks";
import { SUGGESTED_VIEWS, describeQuery, savedViewHref, viewQuery } from "@/lib/ticket-list";
import { Button, IconButton } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { floatPanel } from "./pickers";

const VIEW_NAME: Record<string, string> = { all: "All tickets", active: "Active", backlog: "Backlog", agent_owned: "Agent-owned", needs_human: "Needs human" };
export const describeView = (query: string) => describeQuery(query, { view: (v) => VIEW_NAME[v] ?? v });

const DUPLICATE = "You already have a view with that name";
const errorText = (e: Error | null) => (e instanceof ApiError && e.status === 409 ? DUPLICATE : e?.message);

const menuItem =
  "flex h-8 cursor-default select-none items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover";

function Bookmark({ on, className }: { on?: boolean; className?: string }) {
  return (
    <svg aria-hidden width="12" height="14" viewBox="0 0 12 14" className={cx("flex-none", className)} fill={on ? "currentColor" : "none"} stroke="currentColor" strokeWidth="1.3">
      <path d="M2 1.5h8v11L6 9.7l-4 2.8z" />
    </svg>
  );
}

/** Current /tickets view params, or empty when the list isn't open. */
function useCurrentQuery() {
  const pathname = usePathname();
  const search = useSearchParams();
  return pathname === "/tickets" ? viewQuery(new URLSearchParams(search.toString())) : "";
}

/**
 * Save view dialog: a name, what the view keeps, and suggested views that
 * fill the name and filters. Opens the new view on save.
 */
export function SaveViewDialog({ query, open, onOpenChange }: { query: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const create = useCreateSavedView();
  const router = useRouter();
  const search = useSearchParams();
  const [name, setName] = useState("");
  const [q, setQ] = useState(query);
  const [lastOpen, setLastOpen] = useState(false);
  // Reset the form each time the dialog opens.
  if (open !== lastOpen) {
    setLastOpen(open);
    if (open) {
      setName("");
      setQ(query);
      create.reset();
    }
  }
  function submit(e: FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    create.mutate(
      { name: name.trim(), query: q },
      {
        onSuccess: (v) => {
          onOpenChange(false);
          router.push(savedViewHref(v, search.get("project")));
        },
      },
    );
  }
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby="save-view-desc"
          className="fixed left-1/2 top-[14vh] z-50 w-[min(420px,calc(100%-32px))] -translate-x-1/2 rounded-xl border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)] outline-none"
        >
          <form onSubmit={submit} className="flex flex-col gap-3">
            <Dialog.Title className="type-heading">Save view</Dialog.Title>
            <label className="flex flex-col gap-1.5 text-xs text-fg-secondary">
              Name
              <input
                autoFocus
                value={name}
                maxLength={80}
                onChange={(e) => { setName(e.target.value); create.reset(); }}
                className="h-[30px] rounded-md border border-border-strong bg-surface px-2.5 text-[13px] text-fg outline-none focus:border-accent"
              />
            </label>
            <p id="save-view-desc" className="text-xs text-fg-tertiary">Saves: {describeView(q)}</p>
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-xs text-fg-tertiary">Suggested</span>
              {SUGGESTED_VIEWS.map((s) => (
                <button
                  key={s.name}
                  type="button"
                  title={s.hint}
                  onClick={() => { setName(s.name); setQ(s.query); create.reset(); }}
                  className="h-[22px] rounded-full border border-border-strong px-2.5 text-[11.5px] text-fg-secondary hover:bg-hover"
                >
                  + {s.name}
                </button>
              ))}
            </div>
            {create.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(create.error)}</p>}
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button variant="ghost">Cancel</Button>
              </Dialog.Close>
              <Button type="submit" variant="primary" disabled={!name.trim() || create.isPending}>
                Save view
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** ⋯ menu for a saved view: Rename, Save as new view…, Copy link, Delete view. */
export function SavedViewMenu({
  view,
  onRename,
  onSaveAs,
  children,
}: {
  view: SavedView;
  onRename: () => void;
  onSaveAs: () => void;
  children: ReactNode;
}) {
  const del = useDeleteSavedView();
  const router = useRouter();
  const search = useSearchParams();
  const project = search.get("project");
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={6} className={cx(floatPanel, "w-[210px]")}>
          <DropdownMenu.Item className={menuItem} onSelect={onRename}>
            Rename
            <span className="ml-auto text-[11px] text-fg-tertiary">F2</span>
          </DropdownMenu.Item>
          <DropdownMenu.Item className={menuItem} onSelect={onSaveAs}>Save as new view…</DropdownMenu.Item>
          <DropdownMenu.Item
            className={menuItem}
            onSelect={() => void navigator.clipboard?.writeText(new URL(savedViewHref(view, project), window.location.origin).toString())}
          >
            Copy link
          </DropdownMenu.Item>
          <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
          <DropdownMenu.Item
            className={cx(menuItem, "text-pill-blocked-fg")}
            onSelect={() => {
              if (!window.confirm(`Delete the view “${view.name}”? Tickets aren't changed.`)) return;
              del.mutate(view.id, {
                onSuccess: () => {
                  if (search.get("sv") === view.id) {
                    const next = new URLSearchParams(search.toString());
                    next.delete("sv");
                    router.replace(`/tickets${next.size ? `?${next}` : ""}`);
                  }
                },
              });
            }}
          >
            Delete view
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/** Inline rename: Enter saves, Esc cancels; a duplicate name shows the 409. */
function RenameInput({ view, onDone }: { view: SavedView; onDone: () => void }) {
  const update = useUpdateSavedView();
  const [name, setName] = useState(view.name);
  function save() {
    const next = name.trim();
    if (!next || next === view.name) return onDone();
    update.mutate({ id: view.id, name: next }, { onSuccess: onDone });
  }
  return (
    <div className="flex flex-col gap-0.5">
      <input
        autoFocus
        aria-label={`Rename ${view.name}`}
        value={name}
        maxLength={80}
        onFocus={(e) => e.currentTarget.select()}
        onChange={(e) => { setName(e.target.value); update.reset(); }}
        onKeyDown={(e) => {
          if (e.key === "Enter") { e.preventDefault(); save(); }
          if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onDone(); }
        }}
        onBlur={() => !update.isPending && !update.isError && onDone()}
        className="h-[26px] rounded-[5px] border border-accent bg-surface px-2 text-[12.5px] text-fg outline-none"
      />
      {update.isError && <span role="alert" className="px-1 text-[11.5px] text-pill-blocked-fg">{errorText(update.error)}</span>}
    </div>
  );
}

/** Saved views listed under Tickets in the sidebar, with + Save view. */
export function SidebarSavedViews({ project }: { project?: string }) {
  const views = useSavedViews();
  const pathname = usePathname();
  const search = useSearchParams();
  const current = useCurrentQuery();
  const [renaming, setRenaming] = useState<string | null>(null);
  const [saving, setSaving] = useState<string | null>(null);
  const active = pathname === "/tickets" ? search.get("sv") : null;
  return (
    <div role="group" aria-label="Saved views" className="ml-[15px] flex flex-col gap-px border-l border-border-strong pl-1.5">
      {(views.data ?? []).map((v) =>
        renaming === v.id ? (
          <RenameInput key={v.id} view={v} onDone={() => setRenaming(null)} />
        ) : (
          <div key={v.id} className={cx("group/view flex h-7 items-center rounded-sm pr-1", active === v.id ? "bg-selected" : "hover:bg-hover")}>
            <Link
              href={savedViewHref(v, project)}
              aria-current={active === v.id ? "page" : undefined}
              onKeyDown={(e) => { if (e.key === "F2") { e.preventDefault(); setRenaming(v.id); } }}
              className={cx("flex min-w-0 flex-1 items-center gap-2 px-1.5 text-[12.5px]", active === v.id ? "font-medium text-fg" : "text-fg-secondary hover:text-fg")}
            >
              <Bookmark on={active === v.id} className={active === v.id ? "text-accent" : "text-fg-tertiary"} />
              <span className="truncate">{v.name}</span>
            </Link>
            <SavedViewMenu view={v} onRename={() => setRenaming(v.id)} onSaveAs={() => setSaving(v.query)}>
              <IconButton icon="dots" label={`${v.name} view actions`} size={22} className="opacity-0 focus-visible:opacity-100 group-hover/view:opacity-100 data-[state=open]:opacity-100" />
            </SavedViewMenu>
          </div>
        ),
      )}
      <button
        type="button"
        onClick={() => setSaving(current)}
        className="flex h-7 items-center gap-2 rounded-sm px-1.5 text-left text-xs text-fg-tertiary hover:bg-hover hover:text-fg"
      >
        <Icon name="plus" size={12} />
        Save view
      </button>
      <SaveViewDialog query={saving ?? ""} open={saving !== null} onOpenChange={(o) => !o && setSaving(null)} />
    </div>
  );
}

/**
 * Header for an open saved view: its name, an Edited badge when the list's
 * params differ, Discard / Save changes, and the ⋯ menu.
 */
export function SavedViewHeader({ view }: { view: SavedView }) {
  const router = useRouter();
  const search = useSearchParams();
  const update = useUpdateSavedView();
  const [renaming, setRenaming] = useState(false);
  const [saving, setSaving] = useState(false);
  const now = viewQuery(new URLSearchParams(search.toString()));
  const saved = viewQuery(view.query);
  const edited = now !== saved;
  return (
    <div className="flex min-h-10 flex-wrap items-center gap-2.5 border-b border-border-subtle px-6 py-1.5">
      <Bookmark on className="text-accent" />
      {renaming ? (
        <div className="w-60"><RenameInput view={view} onDone={() => setRenaming(false)} /></div>
      ) : (
        <h2 className="text-sm font-semibold">{view.name}</h2>
      )}
      {edited && <span className="rounded-full bg-border-strong px-2 py-px text-[11px] text-fg-secondary">Edited</span>}
      <span className="min-w-0 flex-1 truncate text-xs text-fg-tertiary">
        {describeView(now)}
        {edited && ` · saved as ${describeView(saved)}`}
      </span>
      {update.isError && <span role="alert" className="text-xs text-pill-blocked-fg">{errorText(update.error)}</span>}
      {edited && (
        <>
          <Button variant="ghost" className="h-[26px]" onClick={() => router.replace(savedViewHref(view, search.get("project")))}>
            Discard
          </Button>
          <Button variant="secondary" className="h-[26px]" disabled={update.isPending} onClick={() => update.mutate({ id: view.id, query: now })}>
            Save changes
          </Button>
        </>
      )}
      <SavedViewMenu view={view} onRename={() => setRenaming(true)} onSaveAs={() => setSaving(true)}>
        <IconButton icon="dots" label={`${view.name} view actions`} bordered size={26} />
      </SavedViewMenu>
      <SaveViewDialog query={now} open={saving} onOpenChange={setSaving} />
    </div>
  );
}
