"use client";

import { Dialog } from "radix-ui";
import { useRouter } from "next/navigation";
import { useDeferredValue, useEffect, useState } from "react";
import { useSearch } from "@/lib/api/hooks";
import { IconButton } from "@/components/ui/button";
import { cx } from "@/lib/cx";
import { NAV_GROUPS, withProject } from "@/lib/project-scope";
import { useRememberedProject } from "./project-scope";

function Match({ text, query }: { text: string; query: string }) {
  const at = query ? text.toLowerCase().indexOf(query.toLowerCase()) : -1;
  return at < 0 ? (
    <>{text}</>
  ) : (
    <>
      {text.slice(0, at)}
      <span className="underline decoration-accent underline-offset-4">
        {text.slice(at, at + query.length)}
      </span>
      {text.slice(at + query.length)}
    </>
  );
}

export function CommandMenu() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [project, setProject] = useState<string>();
  const [scope, setScope] = useState(false);
  const [active, setActive] = useState(0);
  const text = useDeferredValue(query);
  const commands = text.startsWith(">");
  const search = useSearch(commands ? "" : text, scope ? project : undefined);
  const results = (search.data ?? [])
    .map((h) => ({
      id: h.target.id ?? h.target.href ?? h.target.title,
      title: h.target.title,
      meta: h.target.display_id ?? h.target.artifact_type ?? h.target.kind,
      href: h.target.href ?? "/tickets",
      group:
        h.target.kind === "ticket"
          ? "Tickets"
          : h.target.kind === "epic"
            ? "Epics"
            : h.target.kind === "agent"
              ? "Agents"
              : "Artifacts & wiki",
    }))
    .sort(
      (a, b) =>
        ["Tickets", "Artifacts & wiki", "Epics", "Agents"].indexOf(a.group) -
        ["Tickets", "Artifacts & wiki", "Epics", "Agents"].indexOf(b.group),
    );
  const title = query.replace(/^>\s*/, "");
  // Same item set as the sidebar (lib/project-scope.ts), scoped to the remembered project.
  const remembered = useRememberedProject();
  const goTo = NAV_GROUPS.flatMap((g) =>
    g.items.map((i) => ({
      id: `nav-${i.href}`,
      title: `Go to ${i.label}`,
      meta: g.label,
      href: withProject(i.href, remembered),
      group: "Go to",
    })),
  ).filter((i) => (title ? i.title.toLowerCase().includes(title.toLowerCase()) : commands));
  const actions = [
    {
      id: "new",
      title: title ? `Create ticket: ${title}` : "Create ticket",
      meta: "C",
      href: `/tickets/new${title ? `?title=${encodeURIComponent(title)}` : ""}`,
      group: "Actions",
    },
    {
      id: "inbox",
      title: "Review approvals and dispatch ready tickets",
      meta: "Inbox",
      href: "/inbox",
      group: "Actions",
    },
    {
      id: "wiki",
      title: "Search the wiki",
      meta: "Wiki",
      href: "/wiki",
      group: "Actions",
    },
  ];
  const items = [...(commands ? [] : results), ...goTo, ...actions];
  const index = Math.min(active, items.length - 1);
  useEffect(() => {
    if (open)
      document
        .getElementById(`command-${index}`)
        ?.scrollIntoView({ block: "nearest" });
  }, [index, open]);
  const go = (href: string) => {
    setOpen(false);
    router.push(href);
  };
  useEffect(() => {
    const show = () => {
      const currentProject =
        new URLSearchParams(window.location.search).get("project") ??
        window.location.pathname.match(
          /^\/tickets\/([A-Z][A-Z0-9]*)-\d+$/,
        )?.[1];
      setProject(currentProject ?? undefined);
      setScope(!!currentProject);
      setQuery("");
      setActive(0);
      setOpen(true);
    };
    const key = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        show();
      }
    };
    window.addEventListener("keydown", key);
    window.addEventListener("la:search", show);
    return () => {
      window.removeEventListener("keydown", key);
      window.removeEventListener("la:search", show);
    };
  }, []);
  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-scrim" />
        <Dialog.Content
          className="fixed left-1/2 top-[15vh] z-50 flex max-h-[70vh] w-[640px] max-w-[calc(100vw-24px)] -translate-x-1/2 flex-col overflow-hidden rounded-xl border border-border-float bg-surface-raised shadow-xl"
          aria-describedby={undefined}
        >
          <Dialog.Title className="sr-only">Search and commands</Dialog.Title>
          <div className="flex items-center gap-2 border-b border-border px-4 py-3">
            {project && (
              <button
                className="rounded border border-border px-2 text-xs text-fg-secondary"
                onClick={() => setScope(!scope)}
              >
                {scope ? project : "All projects"}
              </button>
            )}
            <input
              autoComplete="off"
              role="combobox"
              aria-label="Search everything"
              aria-expanded="true"
              aria-controls="command-results"
              aria-activedescendant={`command-${index}`}
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setActive(0);
              }}
              placeholder="Search tickets, docs, wiki, agents…"
              className="min-w-0 flex-1 bg-transparent text-sm outline-none"
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  setActive((index + 1) % items.length);
                }
                if (e.key === "ArrowUp") {
                  e.preventDefault();
                  setActive((index + items.length - 1) % items.length);
                }
                if (e.key === "Enter" && items[index]) {
                  e.preventDefault();
                  go(items[index].href);
                }
                if (e.key === "Tab" && project) {
                  e.preventDefault();
                  setScope(!scope);
                }
              }}
            />
            <Dialog.Close asChild>
              <IconButton icon="x" label="Close search" />
            </Dialog.Close>
          </div>
          <div
            id="command-results"
            role="listbox"
            aria-label="Results"
            className="overflow-y-auto p-2"
          >
            {search.isFetching && (
              <p role="status" className="p-3 text-xs text-fg-tertiary">
                Searching…
              </p>
            )}
            {search.isError && (
              <p role="alert" className="p-3 text-xs text-pill-blocked-fg">
                Search failed. Try again.
              </p>
            )}
            {!commands &&
              query &&
              !search.isFetching &&
              results.length === 0 && (
                <p className="p-3 text-sm text-fg-tertiary">
                  No results for “{query}”.
                </p>
              )}
            {items.map((item, i) => (
              <div key={`${item.group}-${item.id}`}>
                {(i === 0 || items[i - 1].group !== item.group) && (
                  <div className="px-3 pb-1 pt-3 text-xs text-fg-tertiary">
                    {item.group}
                  </div>
                )}
                <button
                  type="button"
                  id={`command-${i}`}
                  role="option"
                  aria-selected={i === index}
                  onClick={() => go(item.href)}
                  onMouseMove={() => setActive(i)}
                  className={cx(
                    "flex min-h-[38px] w-full items-center gap-3 rounded-md px-3 text-left text-sm",
                    i === index ? "bg-selected" : "hover:bg-hover",
                  )}
                >
                  <span className="min-w-0 flex-1 truncate">
                    <Match text={item.title} query={title} />
                  </span>
                  <span className="font-mono text-xs text-fg-tertiary">
                    {item.meta}
                  </span>
                </button>
              </div>
            ))}
          </div>
          <div className="border-t border-border px-4 py-2 text-xs text-fg-tertiary">
            ↑↓ navigate · ↵ open · Tab project scope · &gt; commands
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
