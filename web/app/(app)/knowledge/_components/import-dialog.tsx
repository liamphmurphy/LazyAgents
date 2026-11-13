"use client";

import Link from "next/link";
import { useMemo, useRef, useState } from "react";
import { cx } from "@/lib/cx";
import { useCancelImport, useConfirmImport, useImport, useUndoImport, useUploadVault, type ImportJob, type VaultEntry } from "@/lib/api/knowledge-hooks";
import { Button } from "@/components/ui/button";
import { Icon, type IconName } from "@/components/ui/icon";
import { Modal } from "./kit";

type Policy = "skip" | "replace" | "keep_both";
type Filter = "all" | "renamed" | "conflicts" | "unresolved" | "files" | "skipped";

const LIMIT_GB = 2;
const fmt = (n: number) => n.toLocaleString("en");
const mb = (b: number) => (b > 1e9 ? `${(b / 1e9).toFixed(1)} GB` : `${Math.max(1, Math.round(b / 1e6))} MB`);

function entryStatus(e: VaultEntry): { icon: IconName; cls: string; note: string; filter: Filter } {
  if (e.error) return { icon: "x", cls: "text-state-blocked", note: e.error, filter: "skipped" };
  if (e.conflict) return { icon: "alert", cls: "text-state-progress", note: "Already in Knowledge, edited since", filter: "conflicts" };
  if (e.renamed) return { icon: "edit", cls: "text-state-human", note: `Renamed${e.original_title ? ` from “${e.original_title}”` : ""}; the old name is kept as an alias`, filter: "renamed" };
  if (e.unsupported) return { icon: "artifacts", cls: "text-state-review", note: "Not converted; kept as an attachment", filter: "files" };
  if (e.kind === "attachment") return { icon: "library", cls: "text-fg-tertiary", note: "Attachment", filter: "all" };
  return { icon: "check", cls: "text-state-done", note: e.daily ? "Daily note" : (e.tags ?? []).map((t) => `#${t}`).join(" "), filter: "all" };
}

/**
 * Import an Obsidian vault from a zip: upload, read the dry run, choose where
 * and what to do about pages that already exist, import, and undo if needed.
 * Nothing is written until Import.
 */
export function ImportDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  return (
    <Modal open={open} onOpenChange={onOpenChange} title="Import an Obsidian vault" className="top-[6vh] max-h-[88vh] w-[min(940px,calc(100vw-32px))]">
      {/* Closing unmounts the flow, so reopening starts a new import; a running one keeps going on the server. */}
      <ImportFlow onClose={() => onOpenChange(false)} />
    </Modal>
  );
}

function ImportFlow({ onClose }: { onClose: () => void }) {
  const [jobId, setJobId] = useState<string>();
  const job = useImport(jobId).data;
  return (
    <>
      <Steps job={job} />
      {!job ? <Upload onJob={setJobId} /> : <JobView job={job} onClose={onClose} onRetry={() => setJobId(undefined)} />}
    </>
  );
}

function Steps({ job }: { job?: ImportJob }) {
  const at = !job ? 0 : ["queued", "scanning", "preview"].includes(job.state) ? 1 : 2;
  return (
    <ol className="flex items-center gap-3 px-5 pb-1 pt-3 text-13">
      {["Upload", "Review", "Import"].map((s, i) => (
        <li key={s} className="flex flex-1 items-center gap-2 last:flex-none">
          <span className={cx("grid size-[22px] flex-none place-items-center rounded-full text-11 font-bold", i < at ? "bg-state-done text-on-accent" : i === at ? "bg-accent text-on-accent" : "border border-border-strong text-fg-tertiary")}>
            {i < at ? <Icon name="check" size={12} /> : i + 1}
          </span>
          <span className={i <= at ? "font-semibold text-fg" : "text-fg-tertiary"}>{s}</span>
          {i < 2 && <span className="h-px flex-1 bg-border" />}
        </li>
      ))}
    </ol>
  );
}

function Upload({ onJob }: { onJob: (id: string) => void }) {
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File>();
  const [folder, setFolder] = useState("Imported");
  const [drag, setDrag] = useState(false);
  const upload = useUploadVault();
  const tooBig = !!file && file.size > LIMIT_GB * 1e9;
  const pick = (f: File | undefined) => {
    if (f) setFile(f);
    upload.reset();
  };
  return (
    <div className="flex flex-col gap-4 p-5">
      <button
        type="button"
        onClick={() => input.current?.click()}
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={() => setDrag(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDrag(false);
          pick(e.dataTransfer.files[0]);
        }}
        className={cx("flex h-56 flex-col items-center justify-center gap-2 rounded-xl border border-dashed bg-surface-sunken text-center", drag ? "border-accent bg-accent-soft" : "border-[#9B8CFF66]")}
      >
        <span className="grid size-12 place-items-center rounded-full bg-accent-soft text-accent">
          <Icon name="upload" size={22} />
        </span>
        <span className="text-16 font-semibold text-fg">{file ? file.name : "Drop a zipped vault here"}</span>
        <span className="text-13 text-fg-tertiary">
          {file ? mb(file.size) : <><span className="font-semibold text-accent">choose a file</span> · .zip up to {LIMIT_GB} GB</>}
        </span>
        <span className="text-12 text-fg-tertiary">Your .obsidian settings, .trash and plugin data are ignored</span>
      </button>
      <input ref={input} type="file" accept=".zip,application/zip" className="hidden" onChange={(e) => pick(e.target.files?.[0])} />
      <div className="flex flex-wrap items-end gap-4">
        <label className="flex flex-col gap-1.5 text-12 font-semibold text-fg-tertiary">
          Import into
          <span className="flex h-8 w-64 items-center gap-2 rounded-md border border-border-strong bg-surface px-2.5">
            <Icon name="folder" size={14} className="text-fg-secondary" />
            <input value={folder} onChange={(e) => setFolder(e.target.value)} className="flex-1 bg-transparent text-13 font-normal text-fg outline-none" aria-label="Destination folder" />
          </span>
        </label>
        <span className="flex h-8 items-center gap-2 rounded-md bg-accent-soft px-3 text-13 font-medium text-accent">
          <Icon name="lock" size={13} /> Private: you and your personas
        </span>
        <span className="flex-1" />
        <Button
          variant="primary"
          disabled={!file || tooBig || upload.isPending}
          onClick={() => file && upload.mutate({ file, folder: folder.replace(/^\/+|\/+$/g, "") }, { onSuccess: (j) => onJob(j.id) })}
        >
          {upload.isPending ? "Uploading…" : "Prepare dry run"}
        </Button>
      </div>
      {(tooBig || upload.error) && <p className="text-12 text-state-blocked">{tooBig ? `That file is larger than ${LIMIT_GB} GB.` : upload.error?.message}</p>}
      <p className="text-12 text-fg-tertiary">Nothing is written until you review the dry run and import.</p>
    </div>
  );
}

function JobView({ job, onClose, onRetry }: { job: ImportJob; onClose: () => void; onRetry: () => void }) {
  const [filter, setFilter] = useState<Filter>("all");
  const [policy, setPolicy] = useState<Policy>("keep_both");
  const confirm = useConfirmImport();
  const cancel = useCancelImport();
  const undo = useUndoImport();
  const plan = job.plan;
  const entries = useMemo(() => plan?.entries ?? [], [plan]);
  const counts = useMemo(() => {
    const c: Record<Filter, number> = { all: entries.length, renamed: 0, conflicts: 0, unresolved: plan?.unresolved?.length ?? 0, files: 0, skipped: 0 };
    for (const e of entries) {
      const f = entryStatus(e).filter;
      if (f !== "all") c[f]++;
    }
    return c;
  }, [entries, plan]);

  if (job.state === "queued" || job.state === "scanning")
    return <Progress label={`Scanning the archive`} done={job.files_done} total={0} note="Checking every file and resolving links. Nothing is written." />;

  if (job.state === "failed")
    return (
      <div className="flex flex-col gap-4 p-5">
        <div className="rounded-lg border border-border-subtle bg-surface-sunken p-4 text-13 text-fg-2">{job.error || "The import failed."}</div>
        <p className="text-12 text-fg-tertiary">Archives with paths outside the vault, absolute paths, too many files or extreme compression are refused whole. Nothing was written.</p>
        <div className="flex gap-2">
          <Button variant="primary" onClick={onRetry}>Choose another file</Button>
          <Button onClick={onClose}>Close</Button>
        </div>
      </div>
    );

  if (job.state === "queued_commit" || job.state === "importing")
    return <Progress label="Importing" done={job.files_done} total={entries.length} note={`${fmt(job.files_done)} of ${fmt(entries.length)} files`} errors={(job.report ?? []).filter((r) => r.status === "failed" || r.status === "skipped")} />;

  if (job.state === "done" || job.state === "undone") {
    const by = (s: string) => (job.report ?? []).filter((r) => r.status === s).length;
    return (
      <div className="flex flex-col gap-4 p-5">
        <div className="flex items-center gap-3">
          <span className={cx("grid size-10 place-items-center rounded-full", job.state === "done" ? "bg-[#4CB78233] text-state-done" : "bg-selected text-fg-secondary")}>
            <Icon name={job.state === "done" ? "check" : "history"} size={20} />
          </span>
          <div>
            <p className="text-16 font-semibold text-fg">{job.state === "done" ? `Imported ${fmt(by("created") + by("updated"))} files` : "This import was undone"}</p>
            <p className="text-12 text-fg-tertiary">Into {job.folder || "the top level"}/ · {new Date(job.updated_at).toLocaleString("en", { dateStyle: "medium", timeStyle: "short" })}</p>
          </div>
        </div>
        {job.state === "done" && (
          <ul className="space-y-1.5 text-13 text-fg-2">
            {by("unchanged") > 0 && <li>{fmt(by("unchanged"))} unchanged since the last import</li>}
            {by("conflict") > 0 && <li><Icon name="alert" size={13} className="mr-1.5 inline text-state-progress" />{by("conflict")} conflicts kept both, as suggestions to review</li>}
            {by("skipped") + by("failed") > 0 && <li><Icon name="x" size={13} className="mr-1.5 inline text-state-blocked" />{by("skipped") + by("failed")} skipped; their reasons are below</li>}
            {(plan?.unresolved?.length ?? 0) > 0 && <li><Icon name="link" size={13} className="mr-1.5 inline text-state-human" />{plan!.unresolved!.length} unresolved links, listed in <Link href="/knowledge/connect" className="text-accent underline" onClick={onClose}>Keep notes connected</Link></li>}
          </ul>
        )}
        <Failures rows={(job.report ?? []).filter((r) => r.status === "failed" || r.status === "skipped")} />
        {undo.error && <p className="text-12 text-state-blocked">{undo.error.message}</p>}
        <div className="flex items-center gap-2">
          <Button variant="primary" onClick={onClose}>Done</Button>
          {job.state === "done" && (
            <Button icon="history" onClick={() => undo.mutate(job.id)} disabled={undo.isPending}>
              {undo.isPending ? "Undoing…" : "Undo this import"}
            </Button>
          )}
          <span className="text-12 text-fg-tertiary">{job.state === "done" && "Undo removes the pages it created and restores the ones it replaced."}</span>
        </div>
      </div>
    );
  }

  // preview: the dry run
  const shown = entries.filter((e) => filter === "all" || entryStatus(e).filter === filter);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto px-5 pb-4">
        <p className="mt-2 text-13 text-fg-tertiary">
          <b className="text-fg">Dry run</b> · nothing has been written yet
        </p>
        <div className="mt-3 grid grid-cols-5 gap-2">
          {(
            [
              [plan?.notes ?? 0, "notes", "text-fg"],
              [plan?.attachments ?? 0, "attachments", "text-fg"],
              [plan?.tags?.length ?? 0, "tags", "text-fg"],
              [plan?.resolved ?? 0, "links resolved", "text-state-done"],
              [plan?.unresolved?.length ?? 0, "unresolved", "text-state-human"],
            ] as const
          ).map(([n, label, cls]) => (
            <div key={label} className="rounded-lg border border-border-subtle bg-surface p-3">
              <div className={cx("text-22 font-bold", cls)}>{fmt(n)}</div>
              <div className="text-12 text-fg-tertiary">{label}</div>
            </div>
          ))}
        </div>
        <div role="tablist" className="mt-4 flex flex-wrap gap-1">
          {(
            [
              ["all", "All"],
              ["renamed", "Renamed"],
              ["conflicts", "Conflicts"],
              ["unresolved", "Unresolved"],
              ["files", "Kept as files"],
              ["skipped", "Skipped"],
            ] as const
          ).map(([f, label]) => (
            <button key={f} role="tab" aria-selected={filter === f} onClick={() => setFilter(f)} className={cx("h-7 rounded-md px-3 text-12 font-medium", filter === f ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover")}>
              {label} <span className="ml-1 text-fg-tertiary">{fmt(counts[f])}</span>
            </button>
          ))}
        </div>
        <div className="mt-2 rounded-lg border border-border-subtle bg-surface-sunken">
          {filter === "unresolved" ? (
            <ul className="divide-y divide-border-row">
              {(plan?.unresolved ?? []).slice(0, 300).map((u) => (
                <li key={u} className="flex items-center gap-3 px-4 py-2.5 text-13">
                  <Icon name="link" size={14} className="text-state-human" />
                  <span className="text-[#8E86C8] underline">{u}</span>
                  <span className="text-12 text-fg-tertiary">will link once a page with that name exists</span>
                </li>
              ))}
            </ul>
          ) : (
            <ul className="divide-y divide-border-row">
              {shown.slice(0, 300).map((e) => {
                const s = entryStatus(e);
                return (
                  <li key={e.source} className="grid grid-cols-[20px_1fr_1fr] items-center gap-3 px-4 py-2.5">
                    <Icon name={s.icon} size={15} className={s.cls} />
                    <span className="min-w-0">
                      <span className="block truncate text-13 font-medium text-fg">{e.source}</span>
                      {s.note && <span className="block truncate text-12 text-fg-tertiary">{s.note}</span>}
                    </span>
                    <span className="flex min-w-0 items-center gap-2 text-12 text-fg-secondary">
                      <Icon name="chevr" size={12} className="text-fg-tertiary" />
                      <span className="truncate">{e.error ? "skipped" : e.path}</span>
                    </span>
                  </li>
                );
              })}
              {shown.length > 300 && <li className="px-4 py-2.5 text-12 text-fg-tertiary">and {fmt(shown.length - 300)} more</li>}
              {shown.length === 0 && <li className="px-4 py-6 text-center text-12 text-fg-tertiary">Nothing here.</li>}
            </ul>
          )}
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-6">
          <div>
            <p className="mb-1.5 text-12 font-semibold text-fg-tertiary">Into</p>
            <span className="flex h-8 items-center gap-2 rounded-md border border-border-strong bg-surface px-3 text-13 text-fg-2">
              <Icon name="folder" size={14} /> {job.folder || "Top level"}/
            </span>
          </div>
          <div>
            <p className="mb-1.5 text-12 font-semibold text-fg-tertiary">When a page already exists</p>
            <div role="radiogroup" className="flex h-8 rounded-md border border-border-subtle bg-surface-sunken p-0.5">
              {(
                [
                  ["skip", "Skip"],
                  ["replace", "Replace"],
                  ["keep_both", "Keep both"],
                ] as const
              ).map(([p, label]) => (
                <button key={p} role="radio" aria-checked={policy === p} onClick={() => setPolicy(p)} className={cx("w-24 rounded text-12 font-medium", policy === p ? "bg-selected text-fg" : "text-fg-tertiary hover:text-fg-secondary")}>
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div>
            <p className="mb-1.5 text-12 font-semibold text-fg-tertiary">Who can see them</p>
            <span className="flex h-8 items-center gap-2 rounded-md bg-accent-soft px-3 text-13 font-medium text-accent">
              <Icon name="lock" size={13} /> Private: you and your personas
            </span>
          </div>
        </div>
      </div>
      <div className="flex flex-none items-center gap-3 border-t border-border px-5 py-3">
        <Icon name="alert" size={14} className="text-fg-tertiary" />
        <span className="flex-1 text-12 text-fg-tertiary">Nothing is written until you import. You can undo the whole import afterwards.</span>
        {(confirm.error || cancel.error) && <span className="text-12 text-state-blocked">{(confirm.error ?? cancel.error)?.message}</span>}
        <Button onClick={() => cancel.mutate(job.id, { onSuccess: onClose })} disabled={cancel.isPending}>
          Cancel
        </Button>
        <Button variant="primary" onClick={() => confirm.mutate({ id: job.id, policy })} disabled={confirm.isPending}>
          Import {fmt(plan?.notes ?? 0)} notes
        </Button>
      </div>
    </div>
  );
}

function Progress({ label, done, total, note, errors }: { label: string; done: number; total: number; note: string; errors?: { source: string; reason: string }[] }) {
  const pct = total ? Math.min(100, Math.round((done / total) * 100)) : undefined;
  return (
    <div className="flex flex-col gap-3 p-5">
      <div className="flex items-baseline justify-between text-13">
        <span className="font-semibold text-fg">{label}</span>
        {pct !== undefined && <span className="font-bold text-accent">{pct}%</span>}
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-track" role="progressbar" aria-valuenow={pct} aria-label={label}>
        <div className={cx("h-full rounded-full bg-accent", pct === undefined && "w-1/3 animate-pulse")} style={pct !== undefined ? { width: `${Math.max(2, pct)}%` } : undefined} />
      </div>
      <p className="text-12 text-fg-tertiary">{note}</p>
      {!!errors?.length && <Failures rows={errors} />}
      <p className="text-12 text-fg-tertiary">You can close this; the import keeps going.</p>
    </div>
  );
}

function Failures({ rows }: { rows: { source: string; reason: string }[] }) {
  if (!rows.length) return null;
  return (
    <ul className="max-h-40 space-y-1 overflow-y-auto rounded-lg bg-[#EF64611A] p-3 text-12">
      {rows.map((r) => (
        <li key={r.source} className="text-fg-2">
          <b>{r.source}</b>: <span className="text-fg-secondary">{r.reason || "skipped"}</span>
        </li>
      ))}
    </ul>
  );
}
