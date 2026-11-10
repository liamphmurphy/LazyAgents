"use client";

import { useState } from "react";
import { useCancelPending, useCommitMapping, useMappingPreview, usePendingImport, type FinanceImportResult, type FinancePendingImport } from "@/lib/api/finance-hooks";
import { cx } from "@/lib/cx";
import { COLUMN_ROLES, ROLE_LABELS, columnRole, columnValues, dayLabel, money, setColumnRole, type ColumnRole, type CsvMapping } from "@/lib/finance";
import { Button } from "@/components/ui/button";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const select = "h-[28px] rounded-md border border-border-strong bg-surface-sunken px-[6px] text-[12px] text-fg focus:border-accent focus:outline-none";
// After this long without a mapping, say so: the person can map it themselves.
const SLOW_MS = 45_000;

/**
 * The first upload of a CSV layout Vesper doesn't know: its first rows with a role picker over
 * each column, prefilled with a guess and then with the assistant's mapping once it arrives
 * (until the person edits it), and a preview of what the mapping reads from the whole file.
 * Importing saves the mapping, so this bank's next export skips this step.
 */
export function MappingStep({ initial, onImported, onCancelled }: { initial: FinancePendingImport; onImported: (r: FinanceImportResult) => void; onCancelled: () => void }) {
  const pending = usePendingImport(initial);
  const p = pending.data;
  // The person's edits; until the first one, the assistant's mapping (or the guess) shows.
  const [edited, setEdited] = useState<CsvMapping | null>(null);
  const mapping: CsvMapping = edited ?? p.mapping ?? p.prefill;
  const preview = useMappingPreview(p.id, mapping);
  const commit = useCommitMapping();
  const cancel = useCancelPending();
  const update = (next: CsvMapping) => setEdited(next);
  const slow = p.assistant === "asking" && pending.dataUpdatedAt - Date.parse(p.created_at) > SLOW_MS;
  const width = Math.max(p.columns, 1);
  const markerValues = mapping.direction === undefined ? [] : columnValues(p.rows, mapping.header_row, mapping.direction);
  const ready = preview.isSuccess && !preview.isPlaceholderData && !commit.isPending;

  return (
    <div className="mt-[14px] flex flex-col gap-[12px]">
      <AssistantStatus pending={p} slow={slow} edited={!!edited} />

      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full border-collapse text-[12px]">
          <thead>
            <tr className="bg-surface-sunken">
              <th className="w-[34px] px-2 py-[6px] text-left font-normal text-fg-tertiary">Row</th>
              {Array.from({ length: width }, (_, i) => (
                <th key={i} className="px-[6px] py-[6px] text-left font-normal">
                  <select
                    aria-label={`Column ${i + 1}`}
                    className={cx(select, columnRole(mapping, i) !== "skip" && "border-accent/60 text-accent")}
                    value={columnRole(mapping, i)}
                    onChange={(e) => update(setColumnRole(mapping, i, e.target.value as ColumnRole))}
                  >
                    {COLUMN_ROLES.map((r) => (
                      <option key={r} value={r}>
                        {ROLE_LABELS[r]}
                      </option>
                    ))}
                  </select>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {p.rows.map((row, r) => {
              const isHeader = r + 1 === mapping.header_row;
              const above = r + 1 < mapping.header_row;
              return (
                <tr key={r} className={cx("border-t border-border-subtle", isHeader && "bg-accent-soft/40 font-medium", above && "opacity-40")}>
                  <td className="px-2 py-[5px] tabular-nums text-fg-tertiary">{r + 1}</td>
                  {Array.from({ length: width }, (_, i) => (
                    <td key={i} className={cx("max-w-[220px] truncate px-[6px] py-[5px] font-mono text-[11.5px]", columnRole(mapping, i) === "skip" ? "text-fg-tertiary" : "text-fg-2")} title={row[i]}>
                      {row[i] ?? ""}
                    </td>
                  ))}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="flex flex-wrap items-center gap-x-[14px] gap-y-[8px] text-[12px] text-fg-secondary">
        <label className="flex items-center gap-[6px]">
          Column names on row
          <select aria-label="Header row" className={select} value={mapping.header_row} onChange={(e) => update({ ...mapping, header_row: Number(e.target.value) })}>
            <option value={0}>none</option>
            {p.rows.map((_, r) => (
              <option key={r} value={r + 1}>
                {r + 1}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-[6px]">
          Dates
          <select aria-label="Date format" className={select} value={mapping.date_format} onChange={(e) => update({ ...mapping, date_format: e.target.value })}>
            {p.date_formats.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-[6px]">
          <input type="checkbox" checked={!!mapping.decimal_comma} onChange={(e) => update({ ...mapping, decimal_comma: e.target.checked })} />
          Decimal comma (1.234,56)
        </label>
        <label className="flex items-center gap-[6px]">
          <input type="checkbox" checked={!!mapping.flip_sign} onChange={(e) => update({ ...mapping, flip_sign: e.target.checked })} />
          Purchases show as positive
        </label>
      </div>

      {mapping.direction !== undefined && (
        <div className="motion-rise flex flex-wrap items-center gap-[6px] text-[12px] text-fg-secondary" role="group" aria-label="Values that mean money out">
          <span>Money out when the marker is</span>
          {markerValues.map((v) => {
            const on = mapping.debit_values?.includes(v) ?? false;
            return (
              <button
                key={v}
                type="button"
                aria-pressed={on}
                onClick={() => update({ ...mapping, debit_values: on ? (mapping.debit_values ?? []).filter((x) => x !== v) : [...(mapping.debit_values ?? []), v] })}
                className={cx("h-6 rounded-full border px-[9px] font-mono text-[11.5px]", on ? "border-accent/60 bg-accent-soft text-fg" : "border-border-strong text-fg-tertiary hover:text-fg")}
              >
                {v}
              </button>
            );
          })}
        </div>
      )}

      <Preview preview={preview} />

      {(commit.error || cancel.error) && (
        <p role="alert" className="text-[12px] text-pill-blocked-fg">
          {errorText(commit.error ?? cancel.error)}
        </p>
      )}
      <div className="flex items-center justify-end gap-[8px]">
        <span className="mr-auto text-[11px] text-fg-tertiary">Saved for next time: files with this layout import directly.</span>
        <Button variant="secondary" disabled={cancel.isPending} onClick={() => cancel.mutate(p.id, { onSuccess: onCancelled })}>
          Cancel
        </Button>
        <Button variant="primary" disabled={!ready} onClick={() => commit.mutate({ id: p.id, mapping }, { onSuccess: onImported })}>
          {commit.isPending ? "Importing…" : preview.data && preview.isSuccess ? `Import ${preview.data.count} transactions` : "Import"}
        </Button>
      </div>
    </div>
  );
}

function AssistantStatus({ pending, slow, edited }: { pending: FinancePendingImport; slow: boolean; edited: boolean }) {
  const box = "flex items-start gap-[8px] rounded-lg border px-[10px] py-[8px] text-[12.5px]";
  if (pending.assistant === "mapped") {
    return (
      <div className={cx(box, "motion-rise border-accent/40 bg-accent-soft text-fg")} role="status">
        <span aria-hidden className="text-accent">✦</span>
        <span>
          {edited ? "Your assistant mapped the columns; you've changed them since." : "Your assistant mapped the columns. Check the preview, then import."}
          {pending.mapping_note && <span className="block text-fg-secondary">{pending.mapping_note}</span>}
        </span>
      </div>
    );
  }
  if (pending.assistant === "asking") {
    return (
      <div className={cx(box, "border-border-strong text-fg-2")} role="status">
        <span aria-hidden className="mt-[3px] size-3 flex-none animate-spin rounded-full border-[1.5px] border-track border-t-accent" />
        <span>
          This CSV&rsquo;s layout is new, so your assistant is reading its columns from the first {pending.rows.length} rows.
          <span className="block text-fg-tertiary">{slow ? "It's taking a while: you can map them yourself below." : "You can map them yourself below in the meantime."}</span>
        </span>
      </div>
    );
  }
  return (
    <div className={cx(box, "border-border-strong text-fg-2")} role="status">
      <span>This CSV&rsquo;s layout is new. Pick what each column holds; the assistant couldn&rsquo;t be asked.</span>
    </div>
  );
}

function Preview({ preview }: { preview: ReturnType<typeof useMappingPreview> }) {
  if (preview.isError) {
    return (
      <p role="alert" className="rounded-lg border border-pill-blocked-fg/30 bg-pill-blocked-bg px-[10px] py-[8px] text-[12px] text-pill-blocked-fg">
        {errorText(preview.error)}
      </p>
    );
  }
  const d = preview.data;
  if (!d) return <p className="text-[12px] text-fg-tertiary">Reading the file…</p>;
  return (
    <div className={cx("rounded-lg border border-border bg-surface px-[10px] py-[8px] transition-opacity", preview.isPlaceholderData && "opacity-60")} aria-label="Preview">
      <p className="text-[12px] text-fg-secondary">
        Reads <b className="font-semibold text-fg">{d.count}</b> transactions · {dayLabel(d.first_date)} – {dayLabel(d.last_date)} · in <span className="text-pill-done-fg">{money(d.in_cents)}</span> · out{" "}
        <span className="text-pill-blocked-fg">{money(d.out_cents)}</span>
      </p>
      {d.count > 2 && (d.out_cents === 0 || d.in_cents === 0) && (
        <p className="mt-[4px] text-[12px] text-pill-human-fg">
          ⚠ Every amount reads as money {d.out_cents === 0 ? "in" : "out"}. If that&rsquo;s wrong, mark the in/out column, or tick &ldquo;Purchases show as positive&rdquo;.
        </p>
      )}
      <ul className="mt-[6px] flex flex-col gap-[2px]">
        {d.rows.map((r, i) => (
          <li key={i} className="grid grid-cols-[56px_1fr_auto] gap-[10px] text-[12px]">
            <span className="text-fg-tertiary">{dayLabel(r.posted_on)}</span>
            <span className="truncate text-fg-2">{r.description}</span>
            <span className={cx("tabular-nums", r.amount_cents > 0 ? "text-pill-done-fg" : "text-fg-2")}>{money(r.amount_cents, { cents: true, sign: true })}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
