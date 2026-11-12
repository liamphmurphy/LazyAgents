"use client";

import { SetupProgress } from "@/components/domain/setup-progress";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState, type ReactNode, type TransitionEvent } from "react";
import { usePulse } from "@/components/ui/motion";
import { useBootstrap } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { useNavCounts } from "@/lib/use-nav-counts";
import { WORK_ITEMS, isActive, type NavBadge } from "@/lib/project-scope";
import { badgeLabel } from "@/lib/tab-bar";
import { viewedChatId } from "@/lib/persona-dock";
import { Icon, Mark } from "@/components/ui/icon";
import { ChatHistory, NewConversationButton, PersonaRail } from "@/app/(app)/chat/_components/chat-history";
import { SidebarSpaces, SpaceRail } from "@/app/(app)/spaces/_components/spaces-nav";
import { NewSpaceDialog } from "@/app/(app)/spaces/_components/new-space-dialog";
import { HumanAvatar } from "@/components/domain/avatars";
import { AppsMenu } from "./apps-menu";
import { useEditorRail } from "./editor-layout";

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
      <span ref={inbox} className="grid h-[18px] min-w-[18px] place-items-center rounded-[9px] bg-state-human px-[5px] text-xs font-semibold text-approve-fg">
        {n}
        <span className="sr-only"> unread</span>
      </span>
    );
  }
  if (kind === "runs") {
    return (
      <span className="flex items-center gap-[6px] text-xs text-fg-tertiary">
        <span aria-hidden className="size-1.5 rounded-full bg-state-done" />
        {n} running
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
    <span className="font-mono text-xs text-pill-human-fg" title={`${n} suggested edits to review`}>
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

const footerButton = "relative grid size-[28px] place-items-center rounded-md";

/** The bottom of the menu: you, then Customization (personas, agents, memory) and the workspace's Settings. */
function MenuFooter({ active, settingsNeedYou }: { active: (href: string) => boolean; settingsNeedYou: boolean }) {
  const boot = useBootstrap();
  const me = boot.data?.me;
  return (
    <div className="flex items-center gap-2.5 border-t border-border-subtle px-1 pt-2.5">
      {me && <HumanAvatar name={me.name} initials={me.initials} color={me.avatar_color} src={me.avatar_url} size={24} label={false} />}
      <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{me?.name ?? ""}</span>
      <FooterLinks active={active} settingsNeedYou={settingsNeedYou} />
    </div>
  );
}

function FooterLinks({ active, settingsNeedYou, rail }: { active: (href: string) => boolean; settingsNeedYou: boolean; rail?: boolean }) {
  return (
    <div className={cx("flex gap-1", rail && "flex-col")}>
      <Link href="/customization" aria-label="Customization" title="Customization: personas, agents, memory and your profile" aria-current={active("/customization") ? "page" : undefined}
        className={cx(footerButton, rail && "size-[30px]", active("/customization") ? "bg-selected text-fg" : "text-fg-tertiary hover:bg-hover hover:text-fg")}>
        <Icon name="sliders" size={15} />
      </Link>
      <Link href="/settings" title="Settings: credentials, projects and runners" aria-current={active("/settings") ? "page" : undefined}
        className={cx(footerButton, rail && "size-[30px]", active("/settings") ? "bg-selected text-fg" : "text-fg-tertiary hover:bg-hover hover:text-fg")}>
        <Icon name="gear" size={15} />
        <span className="sr-only">Settings{settingsNeedYou && ": a setting runs need is missing"}</span>
        {settingsNeedYou && <span aria-hidden className="absolute right-1 top-1 size-1.5 rounded-full bg-state-human" />}
      </Link>
    </div>
  );
}

/** Work: what agents are doing for you (Inbox, Runs, Tickets), then the Apps row and its flyout. */
function WorkGroup({ counts, active, onNewSpace }: { counts: Record<NavBadge, number>; active: (href: string) => boolean; onNewSpace: () => void }) {
  return (
    <div role="group" aria-label="Work" className="flex flex-col gap-px">
      <span aria-hidden className="px-2 pb-1 text-xs font-medium text-fg-tertiary">Work</span>
      {WORK_ITEMS.map((n) => (
        <NavLink key={n.href} href={n.href} active={active(n.href)}>
          <Icon name={n.icon} />
          <span className="flex-1">{n.label}</span>
          <Badge kind={n.badge} counts={counts} />
        </NavLink>
      ))}
      <AppsMenu counts={counts} onNewSpace={onNewSpace} />
    </div>
  );
}

export function Sidebar() {
  const pathname = usePathname();
  const counts = useNavCounts();
  const [expanded, setExpanded] = useExpanded();
  // An app editor shows the menu as its rail without touching the remembered preference.
  const editorRail = useEditorRail();
  const [creatingSpace, setCreatingSpace] = useState(false);
  // Only a toggle animates, not restoring the remembered state after mount.
  const [toggled, setToggled] = useState(false);
  const toggle = (next: boolean) => { setToggled(true); setExpanded(next); };
  const settle = (e: TransitionEvent<HTMLElement>) => { if (e.target === e.currentTarget && e.propertyName === "width") setToggled(false); };
  const navActive = (href: string) => isActive(pathname, href);
  const viewed = viewedChatId(pathname);
  const settingsNeedYou = counts.settings > 0;
  const newSpace = () => setCreatingSpace(true);
  const searchEvent = () => window.dispatchEvent(new Event("la:search"));
  if (!expanded || editorRail) {
    return (
      <aside aria-label="Main menu" onTransitionEnd={settle} className={cx("flex w-[52px] flex-none flex-col items-center gap-4 border-r border-border-subtle bg-sidebar px-1.5 pb-3.5 pt-3 min-[612px]:sticky min-[612px]:top-0 min-[612px]:h-dvh", toggled && "motion-sidebar")}>
        <span title="Vesper" className="grid size-[30px] place-items-center">
          <Mark />
        </span>
        <div className="flex flex-col items-center gap-1">
          {!editorRail && <ToggleButton expanded={false} onToggle={() => toggle(true)} />}
          <NewConversationButton id={viewed ?? undefined} rail />
          <button
            type="button"
            aria-label="Search"
            title="Search (⌘K)"
            onClick={searchEvent}
            className="grid size-[30px] place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg"
          >
            <Icon name="search" size={14} strokeWidth={1.6} />
          </button>
        </div>
        <nav aria-label="Primary" className="flex min-h-0 w-full flex-1 flex-col items-center gap-4">
          <div className="flex w-full flex-col gap-px">
            {WORK_ITEMS.map((n) => (
              <NavLink key={n.href} label={badgeLabel(n.label, n.badge, counts)} href={n.href} active={navActive(n.href)}>
                <Icon name={n.icon} />
                {n.badge && counts[n.badge] > 0 && <span aria-hidden className={cx("absolute right-1 top-1 size-1.5 rounded-full", n.badge === "inbox" ? "bg-state-human" : "bg-accent")} />}
              </NavLink>
            ))}
            <AppsMenu rail counts={counts} onNewSpace={newSpace} />
          </div>
          <SpaceRail viewed={viewed} />
          <PersonaRail id={viewed ?? undefined} viewed={viewed} />
          <FooterLinks rail active={navActive} settingsNeedYou={settingsNeedYou} />
        </nav>
        <NewSpaceDialog open={creatingSpace} onOpenChange={setCreatingSpace} />
      </aside>
    );
  }
  return (
    <aside aria-label="Main menu" data-expanded onTransitionEnd={settle} className={cx("flex min-w-0 flex-[1_1_232px] flex-col gap-4 border-r min-[800px]:w-[232px] min-[800px]:flex-none border-border-subtle bg-sidebar px-2.5 pb-3.5 pt-3 min-[800px]:sticky min-[800px]:top-0 min-[800px]:h-dvh", toggled && "motion-sidebar")}>
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-[9px] px-1.5 py-[5px]">
            <Mark />
            <span className="text-[14px] font-semibold tracking-[-0.01em]">Vesper</span>
          </span>
          <ToggleButton expanded onToggle={() => toggle(false)} />
        </div>
        <div className="flex items-center gap-2">
          <NewConversationButton id={viewed ?? undefined} />
          <button
            type="button"
            aria-label="Search"
            title="Search (⌘K)"
            onClick={searchEvent}
            className="grid size-[34px] flex-none place-items-center rounded-[9px] border border-border bg-surface text-fg-secondary hover:bg-hover hover:text-fg"
          >
            <Icon name="search" size={15} strokeWidth={1.6} />
          </button>
        </div>
      </div>
      {/* Work and Spaces on top, as in other chat apps; the history fills the rest and scrolls. */}
      <nav aria-label="Primary" className="flex min-h-0 flex-1 flex-col gap-4">
        <WorkGroup counts={counts} active={navActive} onNewSpace={newSpace} />
        <SidebarSpaces viewed={viewed} hideWhenEmpty />
        <ChatHistory id={viewed ?? undefined} viewed={viewed} />
        <SetupProgress />
        <MenuFooter active={navActive} settingsNeedYou={settingsNeedYou} />
      </nav>
      <NewSpaceDialog open={creatingSpace} onOpenChange={setCreatingSpace} />
    </aside>
  );
}
