"use client";

import { SetupProgress } from "@/components/domain/setup-progress";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState, type ReactNode, type TransitionEvent } from "react";
import { cx } from "@/lib/cx";
import { useNavCounts } from "@/lib/use-nav-counts";
import { WORK_ITEMS, isActive } from "@/lib/project-scope";
import { inChats, navigatorArea } from "@/lib/navigator";
import { badgeLabel } from "@/lib/tab-bar";
import { viewedChatId } from "@/lib/persona-dock";
import { Icon, Mark } from "@/components/ui/icon";
import { ChatHistory, NewConversationButton, PersonaRail } from "@/app/(app)/chat/_components/chat-history";
import { SidebarSpaces, SpaceRail } from "@/app/(app)/spaces/_components/spaces-nav";
import { NewSpaceDialog } from "@/app/(app)/spaces/_components/new-space-dialog";
import { AppsMenu } from "./apps-menu";
import { useEditorRail, useReadingFocus } from "./editor-layout";
import { NeedsYou } from "./needs-you";
import { HarnessFooter } from "./harness-footer";
import { useAppearance } from "./theme";

function NavLink({ href, active, label, children }: { href: string; active: boolean; label?: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      aria-label={label}
      title={label}
      className={cx(
        "flex h-[30px] items-center gap-2.5 rounded-sm text-13 font-medium",
        label ? "relative justify-center px-0" : "px-2",
        active ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
      )}
    >
      {children}
    </Link>
  );
}

const EXPANDED_KEY = "lazyagents.sidebar-expanded";

/** The Chats navigator starts hidden, leaving the rail; showing it is remembered per browser (the key predates the navigator). */
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
  const label = expanded ? "Hide chats" : "Show chats";
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

const settingsButton = "relative grid size-[28px] place-items-center rounded-md";

/** Settings live beside the menu toggle, leaving the footer for harness activity. */
function MenuSettings({ active, settingsNeedYou, rail }: { active: (href: string) => boolean; settingsNeedYou: boolean; rail?: boolean }) {
  return (
    <div className={cx("flex gap-1", rail && "flex-col")}>
      <Link href="/customization" aria-label="Customization" title="Customization: personas, agents, memory and your profile" aria-current={active("/customization") ? "page" : undefined}
        className={cx(settingsButton, rail && "size-[30px]", active("/customization") ? "bg-selected text-fg" : "text-fg-tertiary hover:bg-hover hover:text-fg")}>
        <Icon name="sliders" size={15} />
      </Link>
      <Link href="/settings" title="Settings: credentials, projects and runners" aria-current={active("/settings") ? "page" : undefined}
        className={cx(settingsButton, rail && "size-[30px]", active("/settings") ? "bg-selected text-fg" : "text-fg-tertiary hover:bg-hover hover:text-fg")}>
        <Icon name="gear" size={15} />
        <span className="sr-only">Settings{settingsNeedYou && ": a setting runs need is missing"}</span>
        {settingsNeedYou && <span aria-hidden className="absolute right-1 top-1 size-1.5 rounded-full bg-state-human" />}
      </Link>
    </div>
  );
}

/**
 * The Chats navigator beside the rail on chat screens: new chat, spaces, the history and setup
 * progress. Other areas draw their own list in this place (Runs, the Knowledge explorer, an
 * editor's structure), so two lists never stack.
 */
function ChatsNavigator({ viewed, animate, onSettle }: { viewed: string | null; animate: boolean; onSettle: (e: TransitionEvent<HTMLElement>) => void }) {
  return (
    <aside
      aria-label="Chats"
      data-navigator="chats"
      onTransitionEnd={onSettle}
      className={cx("flex min-w-0 flex-[1_1_264px] flex-col gap-4 border-r border-border-subtle bg-sidebar px-2.5 pt-3 min-[800px]:w-[264px] min-[800px]:flex-none min-[800px]:sticky min-[800px]:top-0 min-[800px]:h-dvh", animate && "motion-sidebar")}
    >
      {/* A row: the button grows to the navigator's width, not its height. */}
      <div className="flex flex-none">
        <NewConversationButton id={viewed ?? undefined} />
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-4">
        <SidebarSpaces viewed={viewed} hideWhenEmpty />
        <ChatHistory id={viewed ?? undefined} viewed={viewed} />
        <SetupProgress />
      </div>
    </aside>
  );
}

/**
 * The rail is always there and switches areas (DESIGN.md §3): search, new chat, Needs you, then
 * Chat, Work and the Apps flyout, the persona and space shortcuts, settings, and the harness
 * characters at the foot. On chat screens the toggle shows or hides the Chats navigator beside it,
 * remembered per browser; elsewhere the area brings its own list, or none.
 */
export function Sidebar() {
  const pathname = usePathname();
  const counts = useNavCounts();
  const [appearance] = useAppearance();
  const [expanded, setExpanded] = useExpanded();
  // An app editor draws its own structure panel beside the rail, without touching the remembered preference.
  const editorRail = useEditorRail();
  // The chat's ⌘. reading focus hides the menu altogether, again leaving the preference alone.
  const readingFocus = useReadingFocus();
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
  if (readingFocus) return null;
  const area = editorRail ? null : navigatorArea(pathname);
  const navigator = area !== null && expanded;
  return (
    <>
      <aside aria-label="Main menu" data-expanded={navigator || undefined} className="flex w-[52px] flex-none flex-col items-center gap-4 border-r border-border-subtle bg-sidebar px-1.5 pt-3 min-[612px]:sticky min-[612px]:top-0 min-[612px]:h-dvh">
        {/* Primary spans the foot too, so Settings and Customization stay in the navigation landmark. */}
        <nav aria-label="Primary" className="flex min-h-0 w-full flex-1 flex-col items-center gap-4">
          <span title="Vesper" className="grid size-[30px] place-items-center">
            <Mark />
          </span>
          <div className="flex flex-col items-center gap-1">
            {area && <ToggleButton expanded={navigator} onToggle={() => toggle(!navigator)} />}
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
            <NeedsYou variant="rail" count={counts.inbox} />
          </div>
          <div className="flex min-h-0 w-full flex-1 flex-col items-center gap-4">
            <div className="flex w-full flex-col gap-px">
              <NavLink label="Chat" href="/chat" active={inChats(pathname)}>
                <Icon name="msg" />
              </NavLink>
              {WORK_ITEMS.map((n) => (
                <NavLink key={n.href} label={badgeLabel(n.label, n.badge, counts)} href={n.href} active={navActive(n.href)}>
                  <Icon name={n.icon} />
                  {n.badge && counts[n.badge] > 0 && <span aria-hidden className="absolute right-1 top-1 size-1.5 rounded-full bg-accent" />}
                </NavLink>
              ))}
              <AppsMenu rail counts={counts} onNewSpace={newSpace} />
            </div>
            {/* The navigator lists spaces and personas itself; the rail's shortcuts stand in while it's hidden. */}
            {!navigator && <SpaceRail viewed={viewed} />}
            {!navigator && <PersonaRail id={viewed ?? undefined} viewed={viewed} />}
          </div>
          <MenuSettings rail active={navActive} settingsNeedYou={settingsNeedYou} />
        </nav>
        {appearance.showHarnessActivity !== false && <HarnessFooter />}
        <NewSpaceDialog open={creatingSpace} onOpenChange={setCreatingSpace} />
      </aside>
      {navigator && <ChatsNavigator viewed={viewed} animate={toggled} onSettle={settle} />}
    </>
  );
}
