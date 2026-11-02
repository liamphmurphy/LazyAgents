"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { Icon, Mark, type IconName } from "@/components/ui/icon";
import { Kbd } from "@/components/ui/primitives";
import { HumanAvatar } from "@/components/domain/avatars";

export type SidebarProps = {
  workspace: string;
  inboxCount: number;
  liveAgents: number;
  projects: Array<{ key: string; name: string; color: string }>;
  user: { name: string; initials?: string; role: string };
};

const NAV: Array<{ href: string; label: string; icon: IconName }> = [
  { href: "/", label: "Dashboard", icon: "dashboard" },
  { href: "/inbox", label: "Inbox", icon: "inbox" },
  { href: "/tickets", label: "Tickets", icon: "tickets" },
  { href: "/epics", label: "Epics", icon: "epics" },
  { href: "/agents", label: "Agents", icon: "agents" },
  { href: "/artifacts", label: "Artifacts", icon: "artifacts" },
];

function isActive(pathname: string, href: string) {
  return href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(`${href}/`);
}

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

export function Sidebar({ workspace, inboxCount, liveAgents, projects, user }: SidebarProps) {
  const pathname = usePathname();
  const search = useSearchParams();
  const activeProject = pathname === "/tickets" ? search.get("project") : null;
  return (
    <aside className="flex flex-[1_1_232px] flex-col gap-5 border-r border-border-subtle bg-sidebar px-2.5 pb-3.5 pt-3">
      <div className="flex items-center justify-between gap-2">
        <button type="button" className="flex items-center gap-[9px] rounded-md px-1.5 py-[5px] hover:bg-hover">
          <Mark />
          <span className="text-[13.5px] font-semibold tracking-[-0.01em]">{workspace}</span>
          <Icon name="chev" size={14} strokeWidth={1.6} className="text-fg-tertiary" />
        </button>
        <Link
          href="/tickets/new"
          aria-label="New ticket"
          title="New ticket (C)"
          className="grid size-[30px] place-items-center rounded-md border border-border-strong bg-btn-outline text-fg-2 hover:bg-hover"
        >
          <Icon name="plus" size={15} strokeWidth={1.6} />
        </Link>
      </div>

      {/* ⌘K command menu lands in Phase 6. */}
      <button
        type="button"
        className="flex h-8 items-center gap-[9px] rounded-md border border-border bg-search px-2.5 text-[13px] text-fg-tertiary hover:bg-hover"
      >
        <Icon name="search" size={14} strokeWidth={1.6} />
        <span className="flex-1 text-left">Search…</span>
        <Kbd>⌘K</Kbd>
      </button>

      <nav aria-label="Primary" className="flex flex-col gap-px">
        {NAV.map((n) => (
          <NavLink key={n.href} href={n.href} active={isActive(pathname, n.href) && !(n.href === "/tickets" && activeProject)}>
            <Icon name={n.icon} />
            <span className="flex-1">{n.label}</span>
            {n.icon === "inbox" && inboxCount > 0 && (
              <span className="grid h-[18px] min-w-[18px] place-items-center rounded-[9px] bg-accent px-[5px] text-[11px] font-semibold text-on-accent">
                {inboxCount}
                <span className="sr-only"> unread</span>
              </span>
            )}
            {n.icon === "agents" && liveAgents > 0 && (
              <span className="flex items-center gap-[5px] text-[11.5px] text-fg-tertiary">
                <span aria-hidden className="size-1.5 rounded-full bg-state-done" />
                {liveAgents}
                <span className="sr-only"> running</span>
              </span>
            )}
          </NavLink>
        ))}
      </nav>

      <div className="flex flex-col gap-px">
        <div className="flex items-center justify-between px-2 pb-1.5">
          <span className="text-xs font-medium text-fg-tertiary">Projects</span>
        </div>
        {projects.map((p) => (
          <NavLink key={p.key} href={`/tickets?project=${p.key}`} active={activeProject === p.key}>
            <span aria-hidden className="mx-[3px] size-2.5 flex-none rounded-[3px]" style={{ background: p.color }} />
            <span className="truncate">{p.name}</span>
          </NavLink>
        ))}
      </div>

      <div className="mt-auto flex items-center gap-2.5 px-1.5 py-1">
        <HumanAvatar name={user.name} initials={user.initials} size={26} label={false} />
        <span className="flex flex-1 flex-col leading-tight">
          <span className="font-medium">{user.name}</span>
          <span className="text-[11.5px] text-fg-tertiary">{user.role}</span>
        </span>
        <Link
          href="/settings"
          aria-label="Settings"
          aria-current={isActive(pathname, "/settings") ? "page" : undefined}
          className={cx(
            "grid size-7 place-items-center rounded-sm",
            isActive(pathname, "/settings") ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover",
          )}
        >
          <Icon name="gear" size={15} />
        </Link>
      </div>
    </aside>
  );
}
