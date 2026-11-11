"use client";

import { useEffect, useRef, useState } from "react";
import { useRepositories } from "@/lib/api/repository-hooks";
import { repoShortName } from "@/lib/persona-ui";

export function RepositoryPicker({ value, pending, onPick, disabled, error, signal = 0 }: {
  value?: string; pending?: string; onPick: (repo: string) => void; disabled?: boolean; error?: Error | null; signal?: number;
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const repos = useRepositories(open);
  const [seenSignal, setSeenSignal] = useState(signal);
  if (signal !== seenSignal) { setSeenSignal(signal); setOpen(true); }
  useEffect(() => { if (open) input.current?.focus(); }, [open]);
  const choose = (repo: string) => { onPick(repo); setOpen(false); setText(""); };
  return <div className="relative text-xs" data-testid="repository-picker">
    <button type="button" aria-expanded={open} disabled={disabled} onClick={() => setOpen(!open)} className="rounded-full border border-border bg-surface px-3 py-1 text-fg-secondary hover:bg-hover">
      Repository: {value ? repoShortName(value) : "Choose repository"}{pending && ` → ${repoShortName(pending)} (next turn)`}
    </button>
    {open && <div className="absolute left-0 top-full z-50 mt-2 w-80 rounded-lg border border-border bg-surface p-3 shadow-lg">
      <form onSubmit={(e) => { e.preventDefault(); if (text.trim()) choose(text.trim()); }}>
        <label className="flex flex-col gap-2">Repository name or GitHub URL
          <input ref={input} value={text} onChange={(e) => setText(e.target.value)} placeholder="owner/repo or saved name" className="rounded border border-border bg-background p-2 text-fg" />
        </label>
        <button type="submit" disabled={!text.trim()} className="mt-2 rounded bg-accent px-3 py-1 text-white disabled:opacity-40">Use repository</button>
        <button type="button" onClick={() => setOpen(false)} className="ml-3 text-fg-secondary">Close</button>
      </form>
      {repos.isPending && <p className="mt-2 text-fg-secondary">Loading saved repositories…</p>}
      {repos.error && <p role="alert" className="mt-2 text-pill-blocked-fg">{repos.error.message}</p>}
      <ul aria-label="Available repositories" className="mt-2 max-h-60 overflow-y-auto">
        {(repos.data ?? []).filter((r) => r.name.toLowerCase().includes(text.toLowerCase())).slice(0, 50).map((r) => <li key={r.url}><button type="button" onClick={() => choose(r.url)} className="w-full rounded px-2 py-2 text-left text-fg hover:bg-hover">{r.name}</button></li>)}
      </ul>
      <p className="mt-2 text-fg-tertiary">Use any repository your GitHub credential can access. <a href="/settings#repositories" className="text-accent">Manage saved repos and organizations</a></p>
    </div>}
    {error && <p role="alert" className="mt-1 text-pill-blocked-fg">{error.message}</p>}
  </div>;
}
