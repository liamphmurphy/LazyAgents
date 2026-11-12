"use client";

import { useState } from "react";
import { useCategorize, useFinance, useRefreshFinance, type FinanceImportResult, type FinanceOverview, type FinanceReviewItem } from "@/lib/api/finance-hooks";
import { accountLabel, dayLabel } from "@/lib/finance";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { AccountsDialog } from "./accounts-dialog";
import { AskDrawer, useFinanceAsk } from "./ask-drawer";
import { ImportDialog } from "./import-dialog";
import { Toast, useToast } from "./motion";
import { CashFlow, Kpis, Recurring, Review, Spending } from "./sections";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/**
 * The Finance app (mockup in the Finance chat): a dashboard of the person's imported
 * statements, with an Ask drawer that is a real chat in their Finance space.
 */
export function FinanceScreen() {
  const q = useFinance();
  const data = q.data;
  const [range, setRange] = useState<"6" | "12">("6");
  const [importing, setImporting] = useState(false);
  const [accounts, setAccounts] = useState(false);
  const [askOpen, setAskOpen] = useState(false);
  const ask = useFinanceAsk(data?.space_id);
  const refresh = useRefreshFinance();
  const { toast, show } = useToast();
  const categorize = useCategorize();
  const [saveRules, setSaveRules] = useState(true);
  // Rows sliding out of the review queue while their category saves.
  const [leaving, setLeaving] = useState<Set<string>>(new Set());

  const askAbout = (question: string) => {
    setAskOpen(true);
    ask.ask(question);
  };
  const pick = (item: FinanceReviewItem, category: string) => {
    setLeaving((s) => new Set(s).add(item.id));
    categorize.mutate(
      { id: item.id, category, saveRule: saveRules },
      {
        onSuccess: (r) => {
          if (category === "Transfer") show(<>Marked as a transfer · left out of spending{r.pattern ? <> · future <code className="text-[11px] text-accent">{r.pattern}</code> too</> : null}</>);
          else if (r.pattern) show(<>Rule saved · <code className="text-[11px] text-accent">{r.pattern}</code> → {category}{r.updated ? ` · ${r.updated} past ${r.updated === 1 ? "match" : "matches"} updated` : ""}</>);
          else show(`Categorized as ${category}`);
        },
        onError: (e) => {
          setLeaving((s) => {
            const next = new Set(s);
            next.delete(item.id);
            return next;
          });
          show(errorText(e), "error");
        },
      },
    );
  };
  const imported = (r: FinanceImportResult) => {
    void refresh();
    show(`Imported ${r.import.inserted} new ${r.import.inserted === 1 ? "transaction" : "transactions"}${r.import.transfers ? ` · ${r.import.transfers} transfers linked` : ""}${r.needs_review ? ` · ${r.needs_review} to review` : ""}`);
  };
  // Counts down as rows slide out, before the refetch confirms it.
  const reviewLeft = data ? Math.max(0, data.review_count - data.review.filter((r) => leaving.has(r.id)).length) : 0;

  return (
    <div className="flex h-app flex-col">
      <TopBar
        crumbs={[{ label: "Apps" }, { label: "Finance" }]}
        actions={
          data && data.transaction_count > 0 ? (
            <>
              <Segmented
                label="Months shown"
                small
                value={range}
                onChange={setRange}
                options={[
                  { value: "6", label: "6M" },
                  { value: "12", label: "12M" },
                ]}
              />
              <Button variant="secondary" onClick={() => setAccounts(true)}>
                Accounts
              </Button>
              <Button variant="secondary" icon="upload" onClick={() => setImporting(true)}>
                Import
              </Button>
              <Button variant="primary" onClick={() => setAskOpen(true)}>
                ✦ Ask
              </Button>
            </>
          ) : undefined
        }
      />
      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-[1180px] flex-col gap-[14px] px-6 py-5">
          {q.isError ? (
            <Banner tone="blocked" title="Couldn’t load your finances">
              {errorText(q.error)}
            </Banner>
          ) : !data ? (
            <div aria-busy="true" className="flex flex-col gap-3">
              <div className="grid grid-cols-4 gap-3 max-lg:grid-cols-2">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-[92px]" />
                ))}
              </div>
              <Skeleton className="h-[260px]" />
            </div>
          ) : data.transaction_count === 0 ? (
            <Welcome onImport={() => setImporting(true)} />
          ) : (
            <>
              <Subline data={data} />
              <Kpis data={data} range={Number(range)} reviewLeft={reviewLeft} onBalances={() => setAccounts(true)} />
              <div className="grid grid-cols-[1.6fr_1fr] gap-3 max-lg:grid-cols-1">
                <CashFlow data={data} range={Number(range)} />
                <Spending data={data} onAsk={askAbout} />
              </div>
              <div className="grid grid-cols-[1.25fr_1fr] gap-3 max-lg:grid-cols-1">
                <Review data={data} leaving={leaving} saveRules={saveRules} onSaveRules={setSaveRules} onPick={pick} />
                <Recurring items={data.recurring} />
              </div>
            </>
          )}
        </div>
      </main>
      {data && (
        <>
          <ImportDialog open={importing} onOpenChange={setImporting} accounts={data.accounts} onImported={imported} />
          <AccountsDialog open={accounts} onOpenChange={setAccounts} accounts={data.accounts} />
          <AskDrawer open={askOpen} onOpenChange={setAskOpen} ask={ask} />
        </>
      )}
      <Toast toast={toast} />
    </div>
  );
}

function Subline({ data }: { data: FinanceOverview }) {
  const names = data.accounts.map((a) => accountLabel(a.name, a.mask)).join(", ");
  return (
    <p className="motion-rise text-[12px] text-fg-tertiary">
      {data.accounts.length} {data.accounts.length === 1 ? "account" : "accounts"} · {names}
      {data.last_import && ` · last import ${dayLabel(data.last_import.created_at)}`}
    </p>
  );
}

/** Before the first import: what this is, what to export, and that it stays private. */
function Welcome({ onImport }: { onImport: () => void }) {
  return (
    <section className="motion-rise mx-auto mt-[8vh] flex max-w-[560px] flex-col items-center gap-3 text-center">
      <span aria-hidden className="grid size-12 place-items-center rounded-2xl bg-accent-soft text-accent">
        <Icon name="dollar" size={22} />
      </span>
      <h1 className="text-[20px] font-semibold tracking-[-0.015em] text-fg">Your money, with an assistant that can count</h1>
      <p className="text-[13px] text-fg-secondary">
        Import a CSV or OFX export from your bank or card. Vesper skips anything already imported, pairs card payments with your checking account so nothing counts twice, and learns your categories as you correct them.
      </p>
      <Button variant="primary" icon="upload" onClick={onImport}>
        Import a statement
      </Button>
      <p className="flex items-center gap-[6px] text-[11.5px] text-fg-tertiary">
        <Icon name="lock" size={12} />
        Only you see this, and only Ask, here in Finance, can read it.
      </p>
    </section>
  );
}
