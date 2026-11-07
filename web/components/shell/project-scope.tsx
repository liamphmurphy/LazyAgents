"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useSyncExternalStore } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBootstrap } from "@/lib/api/hooks";
import { PROJECT_ROUTES, resolveProject, withProject, type ProjectMode } from "@/lib/project-scope";
import { ButtonLink } from "@/components/ui/button";
import { EmptyState, Skeleton } from "@/components/ui/primitives";
import type { Crumb } from "./topbar";

type Project = Schemas["Project"];

const STORAGE_KEY = "lazyagents.project";
const CHANGED = "la:project";

function subscribe(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(CHANGED, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(CHANGED, onChange);
  };
}

function read(): string | null {
  try {
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

/** The last project the person used. undefined until it is read on the client. */
export function useRememberedProject(): string | null | undefined {
  return useSyncExternalStore(subscribe, read, () => undefined);
}

/** null means "All projects". */
export function rememberProject(key: string | null) {
  try {
    if (key) window.localStorage.setItem(STORAGE_KEY, key);
    else window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* Storage unavailable (private mode): the URL still carries the project. */
  }
  window.dispatchEvent(new Event(CHANGED));
}

export type ProjectScope =
  | { status: "loading" }
  | { status: "error"; error: Error }
  /** project is undefined on optional routes showing all projects. */
  | { status: "ready"; project?: Project }
  | { status: "chooser"; projects: Project[] }
  | { status: "unavailable"; key: string; archived?: Project; fallback?: Project };

/**
 * The project of a project route: ?project= first, then the remembered project
 * (URL replaced so links and reloads keep it), then a chooser or all projects.
 * skip: the page takes its project from elsewhere (an open chat); no redirect.
 */
export function useProjectScope(mode: ProjectMode, { skip = false }: { skip?: boolean } = {}): ProjectScope {
  const boot = useBootstrap();
  const params = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const remembered = useRememberedProject();
  const requested = params.get("project");
  const projects = boot.data?.projects;
  const resolution = !skip && projects && (requested || remembered !== undefined)
    ? resolveProject(requested, remembered, projects)
    : undefined;
  const visited = resolution?.state === "project" ? resolution.project.key : undefined;
  const redirect = resolution?.state === "redirect" ? resolution.key : undefined;
  const stale = !skip && !!projects && !!remembered && !projects.some((p) => p.key === remembered && !p.archived);

  // Only when the URL's project changes: the switcher clears or replaces the
  // remembered project before it navigates away, and must not be undone here.
  useEffect(() => {
    if (visited && read() !== visited) rememberProject(visited);
  }, [visited]);
  useEffect(() => {
    if (stale) rememberProject(null);
  }, [stale]);
  useEffect(() => {
    if (!redirect) return;
    const next = new URLSearchParams(window.location.search);
    next.set("project", redirect);
    router.replace(`${pathname}?${next}`);
  }, [redirect, pathname, router]);

  if (skip) return { status: "ready" };
  if (boot.isError && !boot.data) return { status: "error", error: boot.error };
  if (!resolution || resolution.state === "redirect") return { status: "loading" };
  if (resolution.state === "project") return { status: "ready", project: resolution.project };
  const active = (projects ?? []).filter((p) => !p.archived);
  if (resolution.state === "unavailable") {
    return {
      status: "unavailable",
      key: resolution.key,
      archived: resolution.archived,
      fallback: active.find((p) => p.key === remembered),
    };
  }
  return mode === "required" ? { status: "chooser", projects: active } : { status: "ready" };
}

/** "■ Payments / Board" (DESIGN.md §3 top bar). */
export function projectCrumbs(project: Project | undefined, label: string): Crumb[] {
  return project
    ? [{ label: project.name, swatch: project.color, href: `/tickets?project=${encodeURIComponent(project.key)}` }, { label }]
    : [{ label }];
}

const WHY: Record<string, string> = {
  "/board": "The board shows one project at a time.",
  "/plans": "A plan proposes tickets for one project.",
  "/chat": "Chats answer from one project's context.",
};

export function ProjectChooser({ base, projects }: { base: string; projects: Project[] }) {
  if (!projects.length) {
    return (
      <EmptyState
        icon="folder"
        title="Create a project first"
        actions={<ButtonLink href="/settings/projects" variant="primary">Create a project</ButtonLink>}
      >
        There are no active projects yet.
      </EmptyState>
    );
  }
  return (
    <section
      aria-labelledby="choose-project"
      className="mx-auto my-12 w-full max-w-[320px] rounded-xl border border-border bg-surface p-5"
    >
      <h2 id="choose-project" className="type-heading">Choose a project</h2>
      {WHY[base] && <p className="mt-1 text-[12.5px] text-fg-secondary">{WHY[base]}</p>}
      <ul className="mt-4 flex flex-col gap-1.5">
        {projects.map((p) => (
          <li key={p.key}>
            <Link
              href={withProject(base, p.key)}
              className="flex h-10 items-center gap-2.5 rounded-md border border-border-strong bg-surface-sunken px-3.5 text-[13px] text-fg hover:bg-hover"
            >
              <span aria-hidden className="size-2.5 flex-none rounded-[3px]" style={{ background: p.color }} />
              <span className="flex-1 truncate">{p.name}</span>
              <span className="font-mono text-[11.5px] text-fg-tertiary">{p.key}</span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function ProjectUnavailable({
  base,
  scope,
}: {
  base: string;
  scope: Extract<ProjectScope, { status: "unavailable" }>;
}) {
  const back = scope.fallback;
  return (
    <EmptyState
      icon="alert"
      title="Project unavailable"
      actions={
        <>
          <ButtonLink variant="primary" href={back ? withProject(base, back.key) : base}>
            {back ? `Back to ${back.name}` : PROJECT_ROUTES[base] === "required" ? "Choose a project" : "All projects"}
          </ButtonLink>
          <ButtonLink variant="outline" href="/settings/projects">Manage projects</ButtonLink>
        </>
      }
    >
      {scope.archived
        ? `${scope.key} is archived. Restore it in Settings to work on it here.`
        : `No project with the key ${scope.key}.`}
    </EmptyState>
  );
}

/** Everything but "ready": render it in place of the page body. */
export function ProjectScopeState({ base, scope }: { base: string; scope: ProjectScope }) {
  switch (scope.status) {
    case "loading":
      return (
        <div role="status" aria-label="Loading project" className="flex flex-col gap-3 p-6">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-24" />
        </div>
      );
    case "error":
      return <p role="alert" className="p-6">Couldn&apos;t load projects: {scope.error.message}</p>;
    case "chooser":
      return <ProjectChooser base={base} projects={scope.projects} />;
    case "unavailable":
      return <ProjectUnavailable base={base} scope={scope} />;
    default:
      return null;
  }
}
