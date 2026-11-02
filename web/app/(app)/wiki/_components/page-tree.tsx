"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import type { WikiPageSummary } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { wikiHref } from "@/lib/wikilinks";
import { Icon } from "@/components/ui/icon";
import { Skeleton } from "@/components/ui/primitives";

type Folder = { name: string; path: string; folders: Map<string, Folder>; pages: WikiPageSummary[]; page?: WikiPageSummary };

function build(pages: WikiPageSummary[]): Folder {
  const root: Folder = { name: "", path: "", folders: new Map(), pages: [] };
  for (const p of pages) {
    const segs = p.path.split("/");
    let f = root;
    segs.slice(0, -1).forEach((s, i) => {
      const key = s.toLowerCase();
      if (!f.folders.has(key)) f.folders.set(key, { name: s, path: segs.slice(0, i + 1).join("/"), folders: new Map(), pages: [] });
      f = f.folders.get(key)!;
    });
    // A page named like a folder ("Payments" next to "Payments/…") becomes
    // that folder's own page.
    const leaf = segs[segs.length - 1].toLowerCase();
    if (f.folders.has(leaf)) f.folders.get(leaf)!.page = p;
    else f.pages.push(p);
  }
  for (const f of root.folders.values()) {
    const own = root.pages.find((p) => p.path.toLowerCase() === f.path.toLowerCase());
    if (own) {
      f.page = own;
      root.pages = root.pages.filter((p) => p !== own);
    }
  }
  return root;
}

const sortBy = <T,>(xs: T[], key: (x: T) => string) => [...xs].sort((a, b) => key(a).localeCompare(key(b)));

function PageLink({ p, current, depth }: { p: WikiPageSummary; current: string; depth: number }) {
  const on = p.path.toLowerCase() === current.toLowerCase();
  return (
    <Link
      href={wikiHref(p.path)}
      aria-current={on ? "page" : undefined}
      className={cx(
        "flex h-7 items-center gap-1.5 rounded-sm pr-2 text-[13px]",
        on ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
      )}
      style={{ paddingLeft: 8 + depth * 12 }}
    >
      <Icon name="artifacts" size={13} className="text-fg-tertiary" />
      <span className="flex-1 truncate">{p.title}</span>
      {p.suggestions > 0 && (
        <span className="font-mono text-[11px] text-pill-human-fg" title={`${p.suggestions} suggested edits`}>
          {p.suggestions}
        </span>
      )}
    </Link>
  );
}

function FolderNode({ f, current, depth }: { f: Folder; current: string; depth: number }) {
  const inside = current.toLowerCase().startsWith(f.path.toLowerCase() + "/") || current.toLowerCase() === f.path.toLowerCase();
  const [open, setOpen] = useState(true);
  const expanded = open || inside;
  return (
    <li>
      <div className="flex items-center" style={{ paddingLeft: depth * 12 }}>
        <button
          type="button"
          aria-expanded={expanded}
          aria-label={`${expanded ? "Collapse" : "Expand"} ${f.name}`}
          onClick={() => setOpen(!expanded)}
          className="grid size-6 flex-none place-items-center rounded-sm text-fg-tertiary hover:bg-hover"
        >
          <Icon name={expanded ? "chev" : "chevr"} size={12} />
        </button>
        {f.page ? (
          <Link
            href={wikiHref(f.page.path)}
            aria-current={f.page.path.toLowerCase() === current.toLowerCase() ? "page" : undefined}
            className={cx(
              "flex h-7 min-w-0 flex-1 items-center rounded-sm px-1 text-[13px] font-medium",
              f.page.path.toLowerCase() === current.toLowerCase() ? "bg-selected text-fg" : "text-fg-2 hover:bg-hover",
            )}
          >
            <span className="truncate">{f.name}</span>
          </Link>
        ) : (
          <span className="flex h-7 min-w-0 flex-1 items-center px-1 text-[13px] font-medium text-fg-2">
            <span className="truncate">{f.name}</span>
          </span>
        )}
      </div>
      {expanded && <TreeList f={f} current={current} depth={depth + 1} />}
    </li>
  );
}

function TreeList({ f, current, depth }: { f: Folder; current: string; depth: number }) {
  return (
    <ul className="flex flex-col">
      {sortBy([...f.folders.values()], (x) => x.name.toLowerCase()).map((c) => (
        <FolderNode key={c.path} f={c} current={current} depth={depth} />
      ))}
      {sortBy(f.pages, (p) => p.title.toLowerCase()).map((p) => (
        <li key={p.id}>
          <PageLink p={p} current={current} depth={depth} />
        </li>
      ))}
    </ul>
  );
}

/**
 * The page tree (PLAN.md §12.6): team pages by folder, then agent
 * namespaces grouped under "Agent memory".
 */
export function PageTree({ pages, current }: { pages: WikiPageSummary[] | undefined; current: string }) {
  const { team, memory } = useMemo(() => {
    const published = (pages ?? []).filter((p) => p.published);
    const isMemory = (p: WikiPageSummary) => p.path.toLowerCase().startsWith("agents/");
    return {
      team: build(published.filter((p) => !isMemory(p))),
      // Drop the "agents/" level: each agent is a folder.
      memory: build(published.filter(isMemory).map((p) => ({ ...p, path: p.path.slice("agents/".length), realPath: p.path }))),
    };
  }, [pages]);

  if (!pages) {
    return (
      <div aria-busy="true" className="flex flex-col gap-2 p-2">
        {[70, 55, 80, 50].map((w) => (
          <Skeleton key={w} className="h-3" style={{ width: `${w}%` }} />
        ))}
      </div>
    );
  }
  const memCurrent = current.toLowerCase().startsWith("agents/") ? current.slice("agents/".length) : "\u0000";

  return (
    <nav aria-label="Wiki pages" className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h2 className="px-2 text-xs font-medium text-fg-tertiary">Pages</h2>
        {team.folders.size + team.pages.length ? (
          <TreeList f={team} current={current} depth={0} />
        ) : (
          <p className="px-2 text-xs text-fg-tertiary">No pages yet.</p>
        )}
      </div>
      {memory.folders.size + memory.pages.length > 0 && (
        <div className="flex flex-col gap-1">
          <h2 className="px-2 text-xs font-medium text-fg-tertiary">Agent memory</h2>
          <MemoryTree f={memory} current={memCurrent} />
        </div>
      )}
    </nav>
  );
}

/** Memory pages carry their real path for links; the tree shows them without "agents/". */
function MemoryTree({ f, current }: { f: Folder; current: string }) {
  const fix = (x: Folder): Folder => ({
    ...x,
    path: x.path ? "agents/" + x.path : x.path,
    page: x.page && withReal(x.page),
    pages: x.pages.map(withReal),
    folders: new Map([...x.folders].map(([k, v]) => [k, fix(v)])),
  });
  return <TreeList f={fix(f)} current={"agents/" + current} depth={0} />;
}

function withReal(p: WikiPageSummary & { realPath?: string }): WikiPageSummary {
  return p.realPath ? { ...p, path: p.realPath } : p;
}
