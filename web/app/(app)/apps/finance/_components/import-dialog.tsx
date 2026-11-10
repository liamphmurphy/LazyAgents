"use client";

import { Dialog } from "radix-ui";
import { useState, type FormEvent, type ReactNode } from "react";
import { useCreateFinanceAccount, useImportStatement, type FinanceAccount, type FinanceImportResult } from "@/lib/api/finance-hooks";
import { cx } from "@/lib/cx";
import { accountLabel, dayLabel } from "@/lib/finance";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const isOFX = (f: File) => /\.(ofx|qfx)$/i.test(f.name);
const NEW = "new";
const KINDS = ["checking", "savings", "credit", "brokerage", "other"] as const;
const field = "h-[34px] rounded-lg border border-border-strong bg-surface-sunken px-[10px] text-[13px] text-fg focus:border-accent focus:outline-none";

/**
 * Import a bank export. An OFX/QFX names its own account; a CSV goes into one the person picks
 * (or adds here). The result lists what really happened, step by step.
 */
export function ImportDialog({ open, onOpenChange, accounts, onImported }: { open: boolean; onOpenChange: (o: boolean) => void; accounts: FinanceAccount[]; onImported: (r: FinanceImportResult) => void }) {
  const upload = useImportStatement();
  const createAccount = useCreateFinanceAccount();
  const [file, setFile] = useState<File | null>(null);
  const [over, setOver] = useState(false);
  const [picked, setPicked] = useState<string>("");
  const [flip, setFlip] = useState(false);
  const [name, setName] = useState("");
  const [kind, setKind] = useState<(typeof KINDS)[number]>("checking");
  const [mask, setMask] = useState("");
  const result = upload.data;
  const accountId = picked || accounts[0]?.id || NEW;
  const needsAccount = !!file && !isOFX(file);
  const busy = upload.isPending || createAccount.isPending;

  const reset = () => {
    setFile(null);
    setPicked("");
    setFlip(false);
    setName("");
    setMask("");
    upload.reset();
    createAccount.reset();
  };
  const close = (o: boolean) => {
    if (!o && result) onImported(result);
    onOpenChange(o);
    if (!o) reset();
  };
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!file || busy) return;
    let account: string | undefined;
    if (needsAccount) {
      if (accountId === NEW) {
        if (!name.trim()) return;
        account = (await createAccount.mutateAsync({ name: name.trim(), kind, ...(mask.trim() ? { mask: mask.trim() } : {}) }).catch(() => undefined))?.id;
        if (!account) return;
        setPicked(account);
      } else {
        account = accountId;
      }
    }
    upload.mutate({ file, accountId: account, flipSign: needsAccount && flip });
  }
  const choose = (f: File | undefined) => {
    if (!f) return;
    upload.reset();
    setFile(f);
  };
  const error = upload.error ?? createAccount.error;

  return (
    <Dialog.Root open={open} onOpenChange={close}>
      <Dialog.Portal>
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          className="motion-dialog fixed left-1/2 top-[10vh] z-50 flex max-h-[80vh] w-[min(480px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-y-auto rounded-2xl border border-border-float bg-surface-raised px-[22px] pb-[18px] pt-[20px] shadow-[var(--shadow-float)] outline-none"
        >
          <div className="flex items-center justify-between">
            <Dialog.Title className="text-[16px] font-semibold text-fg">Import transactions</Dialog.Title>
            <Dialog.Close className="grid size-[28px] place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg" aria-label="Close">
              <Icon name="x" size={14} strokeWidth={1.6} />
            </Dialog.Close>
          </div>
          <p className="mt-1 text-[12px] text-fg-tertiary">A CSV or OFX/QFX export from your bank. Importing overlapping dates is safe: nothing is added twice.</p>

          {result ? (
            <Result result={result} />
          ) : (
            <form onSubmit={submit} className="mt-[14px] flex flex-col gap-[12px]">
              <label
                onDragOver={(e) => {
                  e.preventDefault();
                  setOver(true);
                }}
                onDragLeave={() => setOver(false)}
                onDrop={(e) => {
                  e.preventDefault();
                  setOver(false);
                  choose(e.dataTransfer.files[0]);
                }}
                className={cx(
                  "flex cursor-pointer flex-col items-center gap-1 rounded-xl border-[1.5px] border-dashed px-4 py-[22px] text-center transition-colors",
                  over ? "border-accent bg-accent-soft text-fg" : "border-border-strong text-fg-secondary hover:border-border-hover",
                )}
              >
                <input type="file" accept=".csv,.ofx,.qfx,text/csv" className="sr-only" onChange={(e) => choose(e.target.files?.[0])} />
                <Icon name="upload" size={20} />
                {file ? (
                  <span className="text-[13px] text-fg">
                    {file.name} <span className="text-fg-tertiary">· {Math.max(1, Math.round(file.size / 1024))} KB</span>
                  </span>
                ) : (
                  <span className="text-[13px]">
                    Drop an export here, or <u>choose a file</u>
                  </span>
                )}
                <span className="text-[11px] text-fg-tertiary">{file ? (isOFX(file) ? "OFX files know their account." : "Pick the account this CSV is from.") : "Up to 5 MB"}</span>
              </label>

              {needsAccount && (
                <div className="motion-rise flex flex-col gap-[8px]">
                  <label className="flex flex-col gap-1 text-[12px] text-fg-secondary">
                    <span>Account</span>
                    <select className={field} aria-label="Account" value={accountId} disabled={busy} onChange={(e) => setPicked(e.target.value)}>
                      {accounts.map((a) => (
                        <option key={a.id} value={a.id}>
                          {accountLabel(a.name, a.mask)}
                        </option>
                      ))}
                      <option value={NEW}>New account…</option>
                    </select>
                  </label>
                  {accountId === NEW && (
                    <div className="grid grid-cols-[1fr_120px_80px] gap-[8px]">
                      <input className={field} aria-label="Account name" placeholder="Visa" value={name} maxLength={80} disabled={busy} onChange={(e) => setName(e.target.value)} />
                      <select className={field} aria-label="Kind" value={kind} disabled={busy} onChange={(e) => setKind(e.target.value as (typeof KINDS)[number])}>
                        {KINDS.map((k) => (
                          <option key={k} value={k}>
                            {k[0].toUpperCase() + k.slice(1)}
                          </option>
                        ))}
                      </select>
                      <input className={field} aria-label="Last digits" placeholder="0921" value={mask} maxLength={8} disabled={busy} onChange={(e) => setMask(e.target.value)} />
                    </div>
                  )}
                  <label className="flex items-center gap-2 text-[12px] text-fg-secondary">
                    <input type="checkbox" checked={flip} disabled={busy} onChange={(e) => setFlip(e.target.checked)} />
                    Purchases show as positive amounts (some card exports do)
                  </label>
                </div>
              )}

              {error && (
                <p role="alert" className="text-[12px] text-pill-blocked-fg">
                  {errorText(error)}
                </p>
              )}
              {busy && (
                <div className="h-[3px] overflow-hidden rounded bg-track" aria-label="Importing" role="progressbar">
                  <span className="fin-progress block h-full w-1/3 rounded bg-accent" />
                </div>
              )}
              <div className="flex justify-end gap-[8px]">
                <Dialog.Close asChild>
                  <Button variant="secondary">Cancel</Button>
                </Dialog.Close>
                <Button type="submit" variant="primary" disabled={!file || busy || (needsAccount && accountId === NEW && !name.trim())}>
                  {busy ? "Importing…" : "Import"}
                </Button>
              </div>
            </form>
          )}
          {result && (
            <div className="mt-[16px] flex justify-end">
              <Button variant="primary" onClick={() => close(false)}>
                Done
              </Button>
            </div>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Step({ i, title, detail }: { i: number; title: ReactNode; detail: ReactNode }) {
  return (
    <li className="fin-step grid grid-cols-[18px_1fr] gap-[10px] py-[7px]" style={{ animationDelay: `${i * 140}ms` }}>
      <span aria-hidden className="mt-px grid size-4 place-items-center rounded-full border-[1.5px] border-state-done bg-pill-done-bg text-[10px] text-pill-done-fg">
        ✓
      </span>
      <div>
        <div className="text-[13px] text-fg">{title}</div>
        <div className="text-[12px] text-fg-tertiary">{detail}</div>
      </div>
    </li>
  );
}

/** What the import did, revealed one step at a time. */
function Result({ result }: { result: FinanceImportResult }) {
  const r = result.import;
  const range = result.first_date && result.last_date ? ` · ${dayLabel(result.first_date)} – ${dayLabel(result.last_date)}` : "";
  return (
    <ol className="mt-[14px] flex flex-col" aria-label="Import result">
      <Step i={0} title={`Read ${r.filename}`} detail={`${r.parsed} transactions${range}`} />
      <Step i={1} title={result.account_created ? "Added an account" : "Matched the account"} detail={accountLabel(result.account.name, result.account.mask)} />
      <Step i={2} title="Skipped duplicates" detail={r.duplicates ? `${r.duplicates} already imported` : "None: all new"} />
      <Step i={3} title="Linked transfers" detail={r.transfers ? `${r.transfers} between your accounts, left out of spending` : "None found"} />
      <Step i={4} title="Categorized" detail={`${r.categorized} of ${r.inserted} new${result.needs_review ? ` · ${result.needs_review} need review` : ""}`} />
    </ol>
  );
}
