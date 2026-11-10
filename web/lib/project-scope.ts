import type { IconName } from "@/components/ui/icon";

/**
 * App-shell navigation and project scope (LAZYAGENTS-42). Pure so the rules are
 * unit-tested; the React side lives in components/shell/project-scope.tsx.
 */

export type ScopedProject = { key: string; name: string; color: string; archived: boolean };

/** required: the page needs one project (chooser otherwise). optional: no project means all projects. */
export type ProjectMode = "required" | "optional";

export type NavBadge = "inbox" | "runs" | "wiki" | "settings" | "scheduled";
export type NavItem = { href: string; label: string; icon: IconName; badge?: NavBadge };
export type NavGroup = { label: "Personal" | "Project" | "Workspace" | "Apps"; items: NavItem[] };

/** The one canonical item set: the sidebar and the command menu both read it. */
export const NAV_GROUPS: NavGroup[] = [
  {
    label: "Personal",
    items: [
      { href: "/inbox", label: "Inbox", icon: "inbox", badge: "inbox" },
      { href: "/", label: "Dashboard", icon: "dashboard" },
      { href: "/library", label: "Library", icon: "library" },
      { href: "/workflows", label: "Workflows", icon: "graph" },
      { href: "/scheduled", label: "Scheduled", icon: "clock", badge: "scheduled" },
      { href: "/learn", label: "Learn", icon: "learn" },
      { href: "/customization", label: "Customization", icon: "sliders" },
    ],
  },
  {
    label: "Project",
    items: [
      { href: "/board", label: "Board", icon: "board" },
      { href: "/tickets", label: "Tickets", icon: "tickets" },
      { href: "/epics", label: "Epics", icon: "epics" },
      { href: "/plans", label: "Plans", icon: "edit" },
      { href: "/artifacts", label: "Artifacts", icon: "artifacts" },
    ],
  },
  {
    label: "Workspace",
    items: [
      { href: "/runs", label: "Runs", icon: "play", badge: "runs" },
      { href: "/costs", label: "Costs", icon: "dollar" },
      { href: "/agents", label: "Agents", icon: "agents" },
      { href: "/wiki", label: "Wiki", icon: "wiki", badge: "wiki" },
      { href: "/settings", label: "Settings", icon: "gear", badge: "settings" },
    ],
  },
];

/** Apps: a full page of their own with an Ask drawer, each backed by one of the person's spaces. Chat mode lists them. */
export const APP_GROUP: NavGroup = {
  label: "Apps",
  items: [
    { href: "/apps/docs", label: "Docs", icon: "edit" },
    { href: "/apps/finance", label: "Finance", icon: "dollar" },
  ],
};

export type NavMode = "chat" | "work";

/** Shown in both modes, so opening them leaves the mode alone. */
const PINNED_ROUTES = ["/inbox"];
/** Personal items Work keeps, above Project and Workspace. */
const WORK_PERSONAL = [...PINNED_ROUTES, "/"];
/** Everything else outside this list is Work. */
const CHAT_ROUTES = ["/chat", "/spaces", "/customization", "/library", "/workflows", "/scheduled", "/learn", "/apps"];

/** The mode a page belongs to; undefined for the pinned pages. */
export function routeMode(pathname: string): NavMode | undefined {
  if (PINNED_ROUTES.some((r) => isActive(pathname, r))) return undefined;
  return CHAT_ROUTES.some((r) => isActive(pathname, r)) ? "chat" : "work";
}

/** The page decides; a pinned page keeps the last mode, and Work is the first-run default. */
export function navMode(pathname: string, remembered: NavMode | null | undefined): NavMode {
  return routeMode(pathname) ?? remembered ?? "work";
}

/** Chat keeps Personal minus the Dashboard, then Apps; a conversation is opened from the history. Work keeps Inbox and Dashboard. */
export function navGroupsFor(mode: NavMode): NavGroup[] {
  const [personal, ...work] = NAV_GROUPS;
  if (mode === "chat") return [{ ...personal, items: personal.items.filter((i) => !WORK_PERSONAL.includes(i.href) || PINNED_ROUTES.includes(i.href)) }, APP_GROUP];
  return [{ ...personal, items: personal.items.filter((i) => WORK_PERSONAL.includes(i.href)) }, ...work];
}

/** Where the Chat/Work toggle goes: Chat resumes the newest conversation, Work the last Work page. */
export function modeHref(mode: NavMode, project: string | null | undefined, lastWork: string | null | undefined): string {
  return mode === "chat" ? "/chat" : withProject(lastWork ?? "/board", project);
}

/** Routes whose content belongs to one project, read from ?project=. */
export const PROJECT_ROUTES: Record<string, ProjectMode> = {
  "/board": "required",
  "/plans": "required",
  "/tickets": "optional",
  "/epics": "optional",
  "/artifacts": "optional",
  "/costs": "optional",
};

/** Pages under a project route where switching only updates the remembered project (a draft would be lost). */
const STAY_ON_SWITCH = new Set(["/tickets/new"]);

/** Query parameters that name something inside one project; they don't survive a switch. */
const PROJECT_BOUND_PARAMS = ["project", "epic", "ticket", "id", "artifact", "attach", "attach_title"];

export function isActive(pathname: string, href: string): boolean {
  return href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(`${href}/`);
}

/** The project route a path belongs to, e.g. /chat/123 → /chat. */
export function projectSection(pathname: string): string | undefined {
  return Object.keys(PROJECT_ROUTES).find((base) => isActive(pathname, base));
}

export function withProject(href: string, key: string | null | undefined): string {
  if (!key || !PROJECT_ROUTES[href]) return href;
  return `${href}?project=${encodeURIComponent(key)}`;
}

/**
 * Where to go when the person picks a project (null = All projects) in the switcher.
 * null result: stay on the current page.
 */
export function switchHref(pathname: string, search: URLSearchParams, key: string | null): string | null {
  const base = projectSection(pathname);
  if (!base || STAY_ON_SWITCH.has(pathname)) return null;
  const next = new URLSearchParams(pathname === base ? search : undefined);
  for (const p of PROJECT_BOUND_PARAMS) next.delete(p);
  if (key) next.set("project", key);
  const qs = next.toString();
  return qs ? `${base}?${qs}` : base;
}

/**
 * A URL with filters but no project (/tickets?assignee=…, /epics?epic=…) is a
 * deliberate cross-project link: optional routes honor it instead of narrowing
 * it to the remembered project.
 */
export function hasFilters(search: URLSearchParams): boolean {
  return [...search.keys()].some((k) => k !== "project");
}

export type ProjectResolution<P extends ScopedProject = ScopedProject> =
  /** The URL names an active project. */
  | { state: "project"; project: P }
  /** The URL names an archived (project set) or unknown project. */
  | { state: "unavailable"; key: string; archived?: P }
  /** No project in the URL; replace the URL with the remembered one. */
  | { state: "redirect"; key: string }
  /** No project in the URL and nothing usable remembered. */
  | { state: "none" };

/** URL first, then the remembered project. Archived projects are never resolved. */
export function resolveProject<P extends ScopedProject>(
  requested: string | null | undefined,
  remembered: string | null | undefined,
  projects: P[],
): ProjectResolution<P> {
  if (requested) {
    const found = projects.find((p) => p.key === requested);
    if (found && !found.archived) return { state: "project", project: found };
    return { state: "unavailable", key: requested, archived: found };
  }
  if (remembered && projects.some((p) => p.key === remembered && !p.archived)) {
    return { state: "redirect", key: remembered };
  }
  return { state: "none" };
}

/**
 * The project the switcher shows: the URL's on project routes, nothing on a
 * cross-project link (it lists all projects), else the remembered one.
 */
export function currentProject<P extends ScopedProject>(
  pathname: string,
  search: URLSearchParams,
  remembered: string | null | undefined,
  projects: P[],
): P | undefined {
  const active = projects.filter((p) => !p.archived);
  const section = projectSection(pathname);
  const requested = search.get("project");
  if (section && requested) return active.find((p) => p.key === requested);
  if (section === pathname && PROJECT_ROUTES[section] === "optional" && hasFilters(search)) return undefined;
  return remembered ? active.find((p) => p.key === remembered) : undefined;
}
