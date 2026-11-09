"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState, type ReactNode, type TransitionEvent } from "react";
import { usePulse } from "@/components/ui/motion";
import { DropdownMenu } from "radix-ui";
import { useSecrets } from "@/lib/api/hooks";
import { useScheduledTasks } from "@/lib/api/connector-hooks";
import { tasksNeedingYou } from "@/lib/schedule-ui";
import { cx } from "@/lib/cx";
import { needsAttention } from "@/lib/settings";
import { currentProject, isActive, modeHref, navGroupsFor, switchHref, withProject, type NavBadge, type NavMode, type ScopedProject } from "@/lib/project-scope";
import { viewedChatId } from "@/lib/persona-dock";
import { Icon, Mark } from "@/components/ui/icon";
import { Segmented } from "@/components/ui/controls";
import { Kbd } from "@/components/ui/primitives";
import { floatPanel } from "@/components/domain/pickers";
import { SidebarCapacity } from "@/components/domain/runner-capacity";
import { SidebarSavedViews } from "@/components/domain/saved-views";
import { ChatHistory, NewConversationButton, PersonaRail } from "@/app/(app)/chat/_components/chat-history";
import { useNavMode } from "./nav-mode";
import { rememberProject, useRememberedProject } from "./project-scope";

export type SidebarProps = {
  inboxCount: number;
  /** Ticket and planning runs in progress; chat runs aren't counted. */
  liveRuns: number;
  /** Agent edits to wiki pages waiting for a person (amber: a person acts). */
  wikiSuggestions?: number;
  /** Every project; archived ones are never offered. */
  projects: ScopedProject[];
};

const BADGE_WORD: Partial<Record<NavBadge, string>> = { inbox: "unread", runs: "running" };

/** The rail shows only a dot, so a count has to reach screen readers through the name. */
function railLabel(label: string, badge: NavBadge | undefined, counts: Record<NavBadge, number>) {
  const n = badge ? counts[badge] : 0;
  const word = badge && BADGE_WORD[badge];
  return n > 0 && word ? `${label} ${n} ${word}` : label;
}

function NavLink({ href, active, label, children }: { href: string; active: boolean; label?: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      aria-label={label}
      title={label}
      className={cx(
        "flex h-[30px] items-center gap-2.5 rounded-sm text-[13px] font-medium",
        label ? "relative justify-center px-0" : "px-2",
        active ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
      )}
    >
      {children}
    </Link>
  );
}

function Badge({ kind, counts }: { kind?: NavBadge; counts: Record<NavBadge, number> }) {
  const n = kind ? counts[kind] : 0;
  const inbox = usePulse<HTMLSpanElement>(n);
  if (!kind || n <= 0) return null;
  if (kind === "inbox") {
    return (
      <span ref={inbox} className="grid h-[18px] min-w-[18px] place-items-center rounded-[9px] bg-accent px-[5px] text-[11px] font-semibold text-on-accent">
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

const EXPANDED_KEY = "lazyagents.sidebar-expanded";

/** The main menu starts collapsed to an icon rail; expanding it is remembered per browser. */
function useExpanded(): [boolean, (v: boolean) => void] {
  const [expanded, setExpanded] = useState(false);
  // Read after mount so the server render (always collapsed) matches the first client render.
  useEffect(() => {
    try {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reading browser-only storage after hydration
      if (localStorage.getItem(EXPANDED_KEY) === "1") setExpanded(true);
    } catch {
      /* No storage: stays collapsed. */
    }
  }, []);
  const set = (v: boolean) => {
    setExpanded(v);
    try {
      if (v) localStorage.setItem(EXPANDED_KEY, "1");
      else localStorage.removeItem(EXPANDED_KEY);
    } catch {
      /* Not remembered. */
    }
  };
  return [expanded, set];
}

function ToggleButton({ expanded, onToggle }: { expanded: boolean; onToggle: () => void }) {
  const label = expanded ? "Collapse menu" : "Expand menu";
  return (
    <button
      type="button"
      aria-label={label}
      aria-expanded={expanded}
      title={label}
      onClick={onToggle}
      className="grid size-[30px] flex-none place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg"
    >
      <Icon name="sidebar" size={15} strokeWidth={1.6} />
    </button>
  );
}

/** Half the menu each when expanded, stacked icons on the rail. Switching also goes to that mode's page. */
function ModeToggle({ mode, onChange, rail }: { mode: NavMode; onChange: (m: NavMode) => void; rail?: boolean }) {
  const options = rail
    ? ([
        { value: "chat", name: "Chat", label: <Icon name="msg" /> },
        { value: "work", name: "Work", label: <Icon name="briefcase" /> },
      ] as const)
    : ([
        { value: "chat", label: "Chat" },
        { value: "work", label: "Work" },
      ] as const);
  return <Segmented label="Menu mode" layout={rail ? "column" : "fill"} value={mode} onChange={onChange} options={options} />;
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

export function Sidebar({ inboxCount, liveRuns, wikiSuggestions = 0, projects }: SidebarProps) {
  const pathname = usePathname();
  const search = useSearchParams();
  const remembered = useRememberedProject();
  const active = projects.filter((p) => !p.archived);
  const current = currentProject(pathname, search, remembered, projects);
  const secrets = useSecrets();
  const { mode, lastWork } = useNavMode(pathname);
  // The item only exists in Chat mode, so Work doesn't poll for its badge.
  const scheduled = useScheduledTasks(mode === "chat");
  const counts: Record<NavBadge, number> = { scheduled: tasksNeedingYou(scheduled.data?.tasks), inbox: inboxCount, runs: liveRuns, wiki: wikiSuggestions, settings: needsAttention(secrets.data) ? 1 : 0 };
  const [expanded, setExpanded] = useExpanded();
  // Only a toggle animates, not restoring the remembered state after mount.
  const [toggled, setToggled] = useState(false);
  const toggle = (next: boolean) => { setToggled(true); setExpanded(next); };
  const settle = (e: TransitionEvent<HTMLElement>) => { if (e.target === e.currentTarget && e.propertyName === "width") setToggled(false); };
  const navActive = (href: string) => isActive(pathname, href) && !(href === "/tickets" && search.get("sv"));
  const newTicketHref = current ? `/tickets/new?project=${encodeURIComponent(current.key)}` : "/tickets/new";
  const router = useRouter();
  const switchMode = (m: NavMode) => {
    if (m !== mode) router.push(modeHref(m, current?.key, lastWork));
  };
  const groups = navGroupsFor(mode);
  const chatting = mode === "chat";
  const viewed = viewedChatId(pathname);
  const newTicket = (
    <Link
      href={newTicketHref}
      aria-label="New ticket"
      title="New ticket (C)"
      className="grid size-[30px] flex-none place-items-center rounded-md border border-border-strong bg-btn-outline text-fg-2 hover:bg-hover"
    >
      <Icon name="plus" size={15} strokeWidth={1.6} />
    </Link>
  );
  if (!expanded) {
    return (
      <aside aria-label="Main menu" onTransitionEnd={settle} className={cx("flex w-[52px] flex-none flex-col items-center gap-4 border-r border-border-subtle bg-sidebar px-1.5 pb-3.5 pt-3", toggled && "motion-sidebar", chatting && "min-[612px]:sticky min-[612px]:top-0 min-[612px]:h-dvh")}>
        <span title="Vesper" className="grid size-[30px] place-items-center">
          <Mark />
        </span>
        <div className="flex flex-col items-center gap-1">
          <ToggleButton expanded={false} onToggle={() => toggle(true)} />
          {chatting ? <NewConversationButton id={viewed ?? undefined} rail /> : newTicket}
          <button
            type="button"
            aria-label="Search"
            title="Search (⌘K)"
            onClick={() => window.dispatchEvent(new Event("la:search"))}
            className="grid size-[30px] place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg"
          >
            <Icon name="search" size={14} strokeWidth={1.6} />
          </button>
          <ModeToggle mode={mode} onChange={switchMode} rail />
        </div>
        <nav aria-label="Primary" className="flex w-full flex-col gap-3">
          {groups.map((g) => (
            <div key={g.label} role="group" aria-label={g.label} className="flex flex-col gap-px">
              {g.items.map((n) => (
                <NavLink key={n.href} label={railLabel(n.label, n.badge, counts)} href={withProject(n.href, current?.key)} active={navActive(n.href)}>
                  <Icon name={n.icon} />
                  {n.badge && counts[n.badge] > 0 && <span aria-hidden className="absolute right-1 top-1 size-1.5 rounded-full bg-accent" />}
                </NavLink>
              ))}
            </div>
          ))}
        </nav>
        {chatting && <PersonaRail id={viewed ?? undefined} viewed={viewed} />}
      </aside>
    );
  }
  return (
    <aside aria-label="Main menu" data-expanded onTransitionEnd={settle} className={cx("flex min-w-0 flex-[1_1_232px] flex-col gap-5 border-r min-[800px]:w-[232px] min-[800px]:flex-none border-border-subtle bg-sidebar px-2.5 pb-3.5 pt-3", toggled && "motion-sidebar", chatting && "min-[800px]:sticky min-[800px]:top-0 min-[800px]:h-dvh")}>
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-[9px] px-1.5 py-[5px]">
            <Mark />
            <span className="text-[13.5px] font-semibold tracking-[-0.01em]">Vesper</span>
          </span>
          <div className="flex flex-none items-center gap-1.5">
            {!chatting && newTicket}
            <ToggleButton expanded onToggle={() => toggle(false)} />
          </div>
        </div>
        {!chatting && <ProjectSwitcher projects={active} current={current} />}
      </div>

      <div className="flex flex-col gap-2">
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
        <ModeToggle mode={mode} onChange={switchMode} />
      </div>

      <nav aria-label="Primary" className="flex flex-col gap-3.5">
        {groups.map((g) => (
          <div key={g.label} role="group" aria-label={g.label} className="flex flex-col gap-px">
            {/* Work's pinned items sit above the labelled groups without a heading of their own. */}
            {(chatting || g.label !== "Personal") && <span aria-hidden className="px-2 pb-1 text-xs font-medium text-fg-tertiary">{g.label}</span>}
            {g.items.map((n) => (
              <div key={n.href} className="contents">
                <NavLink href={withProject(n.href, current?.key)} active={navActive(n.href)}>
                  <Icon name={n.icon} />
                  <span className="flex-1">{n.label}</span>
                  <Badge kind={n.badge} counts={counts} />
                </NavLink>
                {n.href === "/tickets" && <SidebarSavedViews project={current?.key} />}
              </div>
            ))}
          </div>
        ))}
      </nav>

      {chatting && <ChatHistory id={viewed ?? undefined} viewed={viewed} />}
      {pathname === "/" && <SidebarCapacity />}
    </aside>
  );
}
