"use client";

import { useState } from "react";
import { Popover } from "radix-ui";
import { Icon } from "@/components/ui/icon";
import { floatPanel } from "@/components/domain/pickers";
import { useRepositories } from "@/lib/api/repository-hooks";
import { cx } from "@/lib/cx";
import { repoShortName, repositoryChoices, sameRepo, type RepoChoice } from "@/lib/persona-ui";
import { composerPill } from "./reply-tools";

const row = "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-hover focus-visible:bg-hover focus-visible:outline-none";
const heading = "px-2 py-1 text-12 font-medium text-fg-tertiary";

function Choices({ title, choices, value, note, onChoose }: {
  title: string; choices: RepoChoice[]; value?: string; note?: (i: number) => string | undefined; onChoose: (url: string) => void;
}) {
  if (!choices.length) return null;
  return <section aria-label={title}>
    <h3 className={heading}>{title}</h3>
    <ul>
      {choices.map((r, i) => <li key={r.url}>
        <button type="button" onClick={() => onChoose(r.url)} aria-current={!!value && sameRepo(value, r.url)} className={row}>
          <span className="min-w-0 flex-1">
            <span className="block truncate">{r.name}</span>
            {note?.(i) && <span className="block text-12 text-fg-tertiary">{note(i)}</span>}
          </span>
          <span className="w-4 flex-none text-accent">{value && sameRepo(value, r.url) && <Icon name="check" size={14} strokeWidth={1.8} />}</span>
        </button>
      </li>)}
    </ul>
  </section>;
}

/**
 * The repository a coding chat works in, as a chip in the composer's option row. It offers the
 * persona's own repositories, then the shared saved ones, and always "Other repository…" for
 * anything else the GitHub credential can reach. The caller shows it only for a persona that can code.
 */
export function RepositoryPicker({ value, pending, onPick, disabled, error, signal = 0, personaName, personaRepos = [] }: {
  value?: string; pending?: string; onPick: (repo: string) => void; disabled?: boolean; error?: Error | null; signal?: number;
  personaName?: string; personaRepos?: string[];
}) {
  const [open, setOpen] = useState(false);
  // "Other repository…": the name or URL box, with the saved repositories filtered by what's typed.
  const [other, setOther] = useState(false);
  const [text, setText] = useState("");
  const repos = useRepositories(open);
  // /repo with nothing after it opens the chip on the box.
  const [seenSignal, setSeenSignal] = useState(signal);
  if (signal !== seenSignal) { setSeenSignal(signal); setOpen(true); setOther(true); }
  const close = () => { setOpen(false); setOther(false); setText(""); };
  const choose = (repo: string) => { onPick(repo); close(); };
  const choices = repositoryChoices(personaRepos, repos.data ?? [], value);
  const label = value ? repoShortName(value) : "Repository";
  const queued = pending ? ` → ${repoShortName(pending)} (next turn)` : "";
  const matching = (repos.data ?? []).filter((r) => r.name.toLowerCase().includes(text.trim().toLowerCase())).slice(0, 50);

  return <div className="relative min-w-0" data-testid="repository-picker">
    <Popover.Root open={open} onOpenChange={(o) => (o ? setOpen(true) : close())}>
      <Popover.Trigger asChild>
        <button type="button" disabled={disabled} aria-label={`Repository: ${value ? repoShortName(value) : "none chosen"}${queued}`} title="The repository this chat works in" className={composerPill(open)}>
          <Icon name="branch" size={13} />
          <span className="truncate">{label}{queued}</span>
          <Icon name="chev" size={13} strokeWidth={1.6} className="flex-none text-fg-tertiary" />
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content side="top" align="start" sideOffset={6} collisionPadding={8} className={cx(floatPanel, "w-80 max-h-[min(26rem,var(--radix-popover-content-available-height))] overflow-y-auto")}>
          {other ? <>
            <form onSubmit={(e) => { e.preventDefault(); if (text.trim()) choose(text.trim()); }} className="flex flex-col gap-2 p-1">
              <label className="flex flex-col gap-1.5 text-12 text-fg-secondary">Repository name or GitHub URL
                <input autoFocus value={text} onChange={(e) => setText(e.target.value)} placeholder="owner/repo or saved name" className="rounded-md border border-border bg-background p-2 text-13 text-fg" />
              </label>
              <div className="flex items-center gap-3">
                <button type="submit" disabled={!text.trim()} className="rounded-md bg-accent px-3 py-1 text-12 text-on-accent disabled:opacity-40">Use repository</button>
                <button type="button" onClick={() => setOther(false)} className="text-12 text-fg-secondary hover:text-fg">Back</button>
              </div>
            </form>
            {repos.isPending && <p className="px-2 py-1 text-12 text-fg-secondary">Loading saved repositories…</p>}
            {repos.error && <p role="alert" className="px-2 py-1 text-12 text-pill-blocked-fg">{repos.error.message}</p>}
            <ul aria-label="Saved repositories" className="mt-1">
              {matching.map((r) => <li key={r.url}><button type="button" onClick={() => choose(r.url)} className={row}><span className="truncate">{r.name}</span></button></li>)}
            </ul>
          </> : <>
            {/* A repo set with /repo that no list holds is still shown, so the current choice is never hidden. */}
            <Choices title="This chat" choices={choices.current ? [choices.current] : []} value={value} onChoose={choose} />
            <Choices title={personaName ? `${personaName}’s repositories` : "Persona repositories"} choices={choices.persona} value={value} note={(i) => (i === 0 ? "Default for new chats" : undefined)} onChoose={choose} />
            {repos.error && <p role="alert" className="px-2 py-1 text-12 text-pill-blocked-fg">{repos.error.message}</p>}
            <Choices title="Saved repositories" choices={choices.saved} value={value} onChoose={choose} />
            <div role="separator" className="my-1 h-px bg-border-subtle" />
            <button type="button" onClick={() => setOther(true)} className={row}>
              <span className="min-w-0 flex-1">
                <span className="block">Other repository…</span>
                <span className="block text-12 text-fg-tertiary">Search saved repositories or paste a URL</span>
              </span>
            </button>
            <p className="px-2 pb-1 pt-2 text-12 text-fg-tertiary">Any repository your GitHub credential can access. <a href="/settings#repositories" className="text-accent">Manage saved repos and owners</a></p>
          </>}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
    {error && !open && <p role="alert" className="absolute bottom-full left-0 mb-1 w-max max-w-[320px] text-12 text-pill-blocked-fg">{error.message}</p>}
  </div>;
}
