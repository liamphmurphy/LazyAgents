"use client";

import Link from "next/link";
import { useMemo } from "react";
import { ago, pageHref } from "@/lib/knowledge";
import type { PageTable } from "@/lib/api/knowledge-hooks";
import { Icon } from "@/components/ui/icon";
import { AUDIENCE, Tag } from "./kit";

/**
 * A saved query's rows as a table: name, the properties the rows share,
 * tags and when each changed. Property columns come from the rows, so a
 * query over NPCs shows role and location without being told to.
 */
export function QueryTable({ table }: { table: PageTable }) {
  const columns = useMemo(() => {
    const seen = new Map<string, number>();
    for (const r of table.rows) for (const k of Object.keys(r.properties ?? {})) if (!["tags", "aliases", "query", "zettel_id", "title"].includes(k)) seen.set(k, (seen.get(k) ?? 0) + 1);
    return [...seen.entries()].sort((a, b) => b[1] - a[1]).slice(0, 4).map(([k]) => k);
  }, [table.rows]);
  if (!table.rows.length) return <p className="rounded-lg border border-border-subtle bg-surface-sunken p-4 text-center text-12 text-fg-tertiary">No pages match this query yet.</p>;
  // A [[link]] value reads as its name, as it does in the properties block.
  const unlink = (s: string) => s.replace(/\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/g, (_, t: string, label?: string) => label ?? t.split("/").pop() ?? t);
  const show = (v: unknown) => (v === undefined || v === null ? "—" : Array.isArray(v) ? v.map(String).map(unlink).join(", ") : typeof v === "object" ? JSON.stringify(v) : unlink(String(v)));
  return (
    <div className="overflow-x-auto rounded-xl border border-border">
      <table className="w-full text-left text-13">
        <thead className="bg-surface-sunken text-12 text-fg-tertiary">
          <tr>
            <th className="px-3 py-2 font-semibold">Name</th>
            {columns.map((c) => (
              <th key={c} className="px-3 py-2 font-semibold">
                {c}
              </th>
            ))}
            <th className="px-3 py-2 font-semibold">tags</th>
            <th className="px-3 py-2 font-semibold">audience</th>
            <th className="px-3 py-2 font-semibold">updated</th>
          </tr>
        </thead>
        <tbody>
          {table.rows.map((r) => {
            const tags = Array.isArray(r.properties?.tags) ? (r.properties.tags as string[]) : [];
            return (
              <tr key={r.id} className="border-t border-border-row hover:bg-hover">
                <td className="px-3 py-2.5">
                  <Link href={pageHref(r.id, r.title)} className="flex items-center gap-2 font-medium text-accent hover:underline">
                    <Icon name="artifacts" size={13} className="text-fg-tertiary" />
                    {r.title}
                  </Link>
                </td>
                {columns.map((c) => (
                  <td key={c} className="px-3 py-2.5 text-fg-2">
                    {show(r.properties?.[c])}
                  </td>
                ))}
                <td className="px-3 py-2.5">
                  <span className="flex flex-wrap gap-1">
                    {tags.slice(0, 3).map((t) => (
                      <Tag key={t}>#{t}</Tag>
                    ))}
                  </span>
                </td>
                <td className="px-3 py-2.5 text-fg-secondary">
                  <span className="flex items-center gap-1.5">
                    <Icon name={AUDIENCE[r.audience].icon} size={12} />
                    {AUDIENCE[r.audience].label}
                  </span>
                </td>
                <td className="px-3 py-2.5 text-fg-tertiary">{ago(r.updated_at)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {table.has_more && <p className="border-t border-border-row px-3 py-2 text-12 text-fg-tertiary">Showing the first {table.rows.length}.</p>}
    </div>
  );
}
