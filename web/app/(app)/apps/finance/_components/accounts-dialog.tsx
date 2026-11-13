"use client";

import { Dialog } from "radix-ui";
import { useState, type FormEvent } from "react";
import { useSetFinanceBalance, type FinanceAccount } from "@/lib/api/finance-hooks";
import { accountLabel, dayLabel, money, parseDollars } from "@/lib/finance";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const field = "h-[32px] rounded-lg border border-border-strong bg-surface-sunken px-[10px] text-13 text-fg focus:border-accent focus:outline-none";

// The person's own calendar day, not UTC: an evening in Portland is already tomorrow in UTC.
const today = () => new Date().toLocaleDateString("en-CA");

/** Each account's newest balance, which net worth rolls through its transactions. */
export function AccountsDialog({ open, onOpenChange, accounts }: { open: boolean; onOpenChange: (o: boolean) => void; accounts: FinanceAccount[] }) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          className="motion-dialog fixed left-1/2 top-[10vh] z-50 flex max-h-[80vh] w-[min(560px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-y-auto rounded-2xl border border-border-float bg-surface-raised px-[22px] pb-[18px] pt-[20px] shadow-[var(--shadow-float)] outline-none"
        >
          <div className="flex items-center justify-between">
            <Dialog.Title className="text-16 font-semibold text-fg">Accounts</Dialog.Title>
            <Dialog.Close className="grid size-[28px] place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg" aria-label="Close">
              <Icon name="x" size={14} strokeWidth={1.6} />
            </Dialog.Close>
          </div>
          <p className="mt-1 text-12 text-fg-tertiary">
            Net worth starts from each account&rsquo;s newest balance. OFX imports record one; for a CSV account, enter what your bank shows. Debts such as a card balance are negative.
          </p>
          {accounts.length === 0 ? (
            <p className="py-6 text-center text-12 text-fg-tertiary">Import a statement first: accounts appear here.</p>
          ) : (
            <ul className="mt-[12px] flex flex-col">
              {accounts.map((a) => (
                <AccountRow key={a.id} account={a} />
              ))}
            </ul>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function AccountRow({ account }: { account: FinanceAccount }) {
  const set = useSetFinanceBalance();
  const [editing, setEditing] = useState(false);
  const [amount, setAmount] = useState("");
  const [asOf, setAsOf] = useState(today);
  const cents = parseDollars(amount);
  function submit(e: FormEvent) {
    e.preventDefault();
    if (cents === null || !asOf) return;
    set.mutate({ accountId: account.id, body: { balance_cents: cents, as_of: asOf } }, { onSuccess: () => setEditing(false) });
  }
  return (
    <li className="border-t border-border-subtle py-[10px] first:border-t-0">
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <div className="truncate text-13 text-fg">{accountLabel(account.name, account.mask)}</div>
          <div className="text-11 text-fg-tertiary">
            {account.kind[0].toUpperCase() + account.kind.slice(1)} · {account.transaction_count} transactions{account.from_ofx ? " · OFX" : ""}
          </div>
        </div>
        <div className="text-right">
          <div className="tabular-nums text-13">{account.balance_cents === undefined ? <span className="text-fg-tertiary">No balance</span> : money(account.balance_cents, { cents: true })}</div>
          {account.balance_as_of && <div className="text-11 text-fg-tertiary">as of {dayLabel(account.balance_as_of)}</div>}
        </div>
        {!editing && (
          <Button variant="secondary" onClick={() => setEditing(true)}>
            {account.balance_cents === undefined ? "Add balance" : "Update"}
          </Button>
        )}
      </div>
      {editing && (
        <form onSubmit={submit} className="motion-rise mt-[8px] flex flex-wrap items-center gap-[8px]">
          <input className={`${field} w-[140px]`} aria-label="Balance" placeholder="-454.67" inputMode="decimal" autoFocus value={amount} onChange={(e) => setAmount(e.target.value)} />
          <input className={field} aria-label="As of" type="date" value={asOf} max={today()} onChange={(e) => setAsOf(e.target.value)} />
          <Button type="submit" variant="primary" disabled={cents === null || set.isPending}>
            {set.isPending ? "Saving…" : "Save"}
          </Button>
          <Button variant="secondary" onClick={() => setEditing(false)}>
            Cancel
          </Button>
          {amount && cents === null && <span className="text-12 text-pill-blocked-fg">Enter an amount like 1234.56</span>}
          {set.isError && <span role="alert" className="text-12 text-pill-blocked-fg">{errorText(set.error)}</span>}
        </form>
      )}
    </li>
  );
}
