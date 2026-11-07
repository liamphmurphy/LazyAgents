"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import type { ReactNode } from "react";
import { DropdownMenu } from "radix-ui";
import { useSecrets } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { needsAttention } from "@/lib/settings";
import { NAV_GROUPS, currentProject, isActive, switchHref, withProject, type NavBadge, type ScopedProject } from "@/lib/project-scope";
import { Icon, Mark } from "@/components/ui/icon";
import { Kbd } from "@/components/ui/primitives";
import { floatPanel } from "@/components/domain/pickers";
import { SidebarCapacity } from "@/components/domain/runner-capacity";
import { HumanAvatar } from "@/components/domain/avatars";
import { rememberProject, useRememberedProject } from "./project-scope";

export type SidebarProps = {
  workspace: string;
  inboxCount: number;
  /** Runs in progress on the runners. */
  liveRuns: number;
  /** Agent edits to wiki pages waiting for a person (amber: a person acts). */
  wikiSuggestions?: number;
  /** Every project; archived ones are never offered. */
  projects: ScopedProject[];
  user: { name: string; initials?: string; role: string };
};

function NavLink({ href, active, children }: { href: string; active: boolean; children: ReactNode }) {
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      className={cx(
        "flex h-[30px] items-center gap-2.5 rounded-sm px-2 text-[13px] font-medium",
        active ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
      )}
    >
      {children}
    </Link>
  );
}

function Badge({ kind, counts }: { kind?: NavBadge; counts: Record<NavBadge, number> }) {
  const n = kind ? counts[kind] : 0;
  if (!kind || n <= 0) return null;
  if (kind === "inbox") {
    return (
      <span className="grid h-[18px] min-w-[18px] place-items-center rounded-[9px] bg-accent px-[5px] text-[11px] font-semibold text-on-accent">
        {n}
        <span className="sr-only"> unread</span>
      </span>
    );
  }
  if (kind === "runs") {
    return (
      <span className="flex items-center gap-[5px] text-[11.5px] text-fg-tertiary">
        <span aria-hidden className="size-1.5 rounded-full bg-state-done" />
        {n}
        <span className="sr-only"> running</span>
      </span>
    );
  }
  if (kind === "settings") {
    return (
      <span className="size-2 rounded-full bg-state-human" title="A setting runs need is missing">
        <span className="sr-only">: a setting runs need is missing</span>
      </span>
    );
  }
  return (
    <span className="font-mono text-[11.5px] text-pill-human-fg" title={`${n} suggested edits to review`}>
      {n}
      <span className="sr-only"> suggested edits to review</span>
    </span>
  );
}

const menuItem =
  "flex h-8 cursor-default select-none items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover";

function ProjectSwitcher({ projects, current }: { projects: ScopedProject[]; current?: ScopedProject }) {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  function pick(value: string) {
    const key = value || null;
    const href = switchHref(pathname, new URLSearchParams(search.toString()), key);
    // A chosen project is remembered when its route opens (useProjectScope);
    // remembering first would also fire the chooser page's own redirect.
    // "All projects" must be forgotten before navigating, or the bare route
    // would redirect back to the old project.
    if (!href || !key) rememberProject(key);
    if (href) router.push(href);
  }
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={`Switch project, current: ${current?.name ?? "All projects"}`}
          className="flex h-9 items-center gap-2.5 rounded-md border border-border-strong bg-surface px-3 text-left hover:bg-hover data-[state=open]:border-accent"
        >
          {current ? (
            <span aria-hidden className="size-2.5 flex-none rounded-[3px]" style={{ background: current.color }} />
          ) : (
            <Icon name="dashboard" size={12} className="flex-none text-fg-tertiary" />
          )}
          <span className="flex-1 truncate text-[13px] font-semibold">{current?.name ?? "All projects"}</span>
          {current && <span className="font-mono text-[11.5px] text-fg-tertiary">{current.key}</span>}
          <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" className="flex-none text-fg-tertiary">
            <path d="m3 4.5 3-3 3 3M3 7.5l3 3 3-3" />
          </svg>
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "w-[260px]")}>
          <DropdownMenu.RadioGroup value={current?.key ?? ""} onValueChange={pick}>
            <DropdownMenu.RadioItem value="" className={menuItem}>
              <Icon name="dashboard" size={12} className="text-fg-secondary" />
              <span className="flex-1">All projects</span>
              <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
            </DropdownMenu.RadioItem>
            <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />
            {projects.map((p) => (
              <DropdownMenu.RadioItem key={p.key} value={p.key} className={menuItem}>
                <span aria-hidden className="size-2.5 flex-none rounded-[3px]" style={{ background: p.color }} />
                <span className="flex-1 truncate">{p.name}</span>
                <span className="font-mono text-[11.5px] text-fg-tertiary">{p.key}</span>
                <span className="grid w-3.5 place-items-center">
                  <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
                </span>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
          <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />
          <DropdownMenu.Item asChild className={menuItem}>
            <Link href="/settings/projects">
              <Icon name="gear" size={12} className="text-fg-secondary" />
              Manage projects
            </Link>
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function Sidebar({ workspace, inboxCount, liveRuns, wikiSuggestions = 0, projects, user }: SidebarProps) {
  const pathname = usePathname();
  const search = useSearchParams();
  const remembered = useRememberedProject();
  const active = projects.filter((p) => !p.archived);
  const current = currentProject(pathname, search, remembered, projects);
  const secrets = useSecrets();
  const counts: Record<NavBadge, number> = { inbox: inboxCount, runs: liveRuns, wiki: wikiSuggestions, settings: needsAttention(secrets.data) ? 1 : 0 };
  return (
    <aside className="flex flex-[1_1_232px] flex-col gap-5 border-r border-border-subtle bg-sidebar px-2.5 pb-3.5 pt-3">
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <button type="button" className="flex items-center gap-[9px] rounded-md px-1.5 py-[5px] hover:bg-hover">
            <Mark />
            <span className="text-[13.5px] font-semibold tracking-[-0.01em]">{workspace}</span>
            <Icon name="chev" size={14} strokeWidth={1.6} className="text-fg-tertiary" />
          </button>
          <Link
            href={current ? `/tickets/new?project=${encodeURIComponent(current.key)}` : "/tickets/new"}
            aria-label="New ticket"
            title="New ticket (C)"
            className="grid size-[30px] place-items-center rounded-md border border-border-strong bg-btn-outline text-fg-2 hover:bg-hover"
          >
            <Icon name="plus" size={15} strokeWidth={1.6} />
          </Link>
        </div>
        <ProjectSwitcher projects={active} current={current} />
      </div>

      {/* Shared command menu also opens from the keyboard. */}
      <button
        type="button"
        onClick={() => window.dispatchEvent(new Event("la:search"))}
        className="flex h-8 items-center gap-[9px] rounded-md border border-border bg-search px-2.5 text-[13px] text-fg-tertiary hover:bg-hover"
      >
        <Icon name="search" size={14} strokeWidth={1.6} />
        <span className="flex-1 text-left">Search…</span>
        <Kbd>⌘K</Kbd>
      </button>

      <nav aria-label="Primary" className="flex flex-col gap-3.5">
        {NAV_GROUPS.map((g) => (
          <div key={g.label} role="group" aria-labelledby={`nav-${g.label}`} className="flex flex-col gap-px">
            <span id={`nav-${g.label}`} className="px-2 pb-1 text-xs font-medium text-fg-tertiary">{g.label}</span>
            {g.items.map((n) => (
              <NavLink key={n.href} href={withProject(n.href, current?.key)} active={isActive(pathname, n.href)}>
                <Icon name={n.icon} />
                <span className="flex-1">{n.label}</span>
                <Badge kind={n.badge} counts={counts} />
              </NavLink>
            ))}
          </div>
        ))}
      </nav>

      {pathname === "/" && <SidebarCapacity />}
      <div className={cx("flex items-center gap-2.5 px-1.5 py-1", pathname !== "/" && "mt-auto")}>
        <HumanAvatar name={user.name} initials={user.initials} size={26} label={false} />
        <span className="flex flex-1 flex-col leading-tight">
          <span className="font-medium">{user.name}</span>
          <span className="text-[11.5px] text-fg-tertiary">{user.role}</span>
        </span>
      </div>
    </aside>
  );
}
