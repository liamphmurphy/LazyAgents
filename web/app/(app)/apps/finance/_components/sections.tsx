"use client";

import { useState, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import type { FinanceOverview, FinanceReviewItem } from "@/lib/api/finance-hooks";
import { cx } from "@/lib/cx";
import { accountLabel, axisMoney, cashBars, cashNote, dayLabel, money, monthLabel, netWorthSeries, niceScale, ordinal, savingsSeries, spendDelta } from "@/lib/finance";
import { Toggle } from "@/components/ui/controls";
import { CountUp, Sparkline } from "./motion";

const GREEN = "var(--state-done)";
const RED = "var(--state-blocked)";
const ACCENT = "var(--accent)";

export function Panel({ title, aside, children, className, delay = 0 }: { title: ReactNode; aside?: ReactNode; children: ReactNode; className?: string; delay?: number }) {
  return (
    <section className={cx("motion-rise relative min-w-0 rounded-xl border border-border bg-surface p-[14px]", className)} style={{ animationDelay: `${delay}ms` }}>
      <div className="mb-[10px] flex items-center gap-2">
        <h2 className="type-heading text-fg">{title}</h2>
        {aside && <div className="ml-auto flex items-center gap-[10px] text-12 text-fg-tertiary">{aside}</div>}
      </div>
      {children}
    </section>
  );
}

function Pill({ tone, children }: { tone: "good" | "warn" | "neutral"; children: ReactNode }) {
  const cls = tone === "good" ? "bg-pill-done-bg text-pill-done-fg" : tone === "warn" ? "bg-pill-human-bg text-pill-human-fg" : "bg-pill-neutral-bg text-pill-neutral-fg";
  return <span className={cx("rounded-full px-[7px] py-[2px] text-11 font-medium", cls)}>{children}</span>;
}

function Tile({ label, children, foot, delay }: { label: string; children: ReactNode; foot: ReactNode; delay: number }) {
  return (
    <div className="motion-rise min-w-0 rounded-xl border border-border bg-surface p-[14px]" style={{ animationDelay: `${delay}ms` }}>
      <div className="text-12 text-fg-tertiary">{label}</div>
      <div className="mt-1 text-22 font-semibold tabular-nums tracking-[-0.02em] text-fg">{children}</div>
      <div className="mt-[6px] flex items-end justify-between gap-2">{foot}</div>
    </div>
  );
}

/** Net worth, this month so far, savings rate and the review queue's size. */
export function Kpis({ data, range, reviewLeft, onBalances }: { data: FinanceOverview; range: number; reviewLeft: number; onBalances: () => void }) {
  const months = data.months.slice(-range);
  const current = data.months.at(-1)!;
  const mtd = current.inflow_cents - current.outflow_cents;
  const sinceLast = data.net_worth_change_cents;
  const rateIn = months.reduce((s, m) => s + m.inflow_cents, 0);
  const rate = rateIn > 0 ? (rateIn - months.reduce((s, m) => s + m.outflow_cents, 0)) / rateIn : undefined;
  const categorized = data.transaction_count ? (data.transaction_count - reviewLeft) / data.transaction_count : 1;
  return (
    <section aria-label="Summary" className="grid grid-cols-4 gap-3 max-lg:grid-cols-2">
      <Tile
        label="Net worth"
        delay={40}
        foot={
          data.net_worth_cents === undefined ? (
            <button type="button" onClick={onBalances} className="text-12 text-accent hover:underline">
              Add account balances
            </button>
          ) : (
            <>
              {sinceLast !== undefined ? <Pill tone={sinceLast >= 0 ? "good" : "warn"}>{money(sinceLast, { sign: true })} this month</Pill> : <span />}
              <Sparkline values={netWorthSeries(months)} color={GREEN} />
            </>
          )
        }
      >
        {data.net_worth_cents === undefined ? <span className="text-fg-tertiary">—</span> : <CountUp value={data.net_worth_cents} format={(n) => money(n)} />}
      </Tile>
      <Tile
        label={`${monthLabel(current.month, "long")} so far`}
        delay={90}
        foot={
          <>
            <span className="text-11 text-fg-tertiary">
              in {money(current.inflow_cents)} · out {money(current.outflow_cents)}
            </span>
            <Sparkline values={months.map((m) => m.inflow_cents - m.outflow_cents)} color={ACCENT} />
          </>
        }
      >
        <CountUp value={mtd} format={(n) => money(n, { sign: true })} />
      </Tile>
      <Tile
        label={`Savings rate · ${range} mo`}
        delay={140}
        foot={
          <>
            <span className="text-11 text-fg-tertiary">of money in, kept</span>
            <Sparkline values={savingsSeries(months)} color="var(--state-review)" />
          </>
        }
      >
        {rate === undefined ? <span className="text-fg-tertiary">—</span> : <CountUp value={rate * 100} format={(n) => `${Math.round(n)}%`} />}
      </Tile>
      <Tile
        label="Needs review"
        delay={190}
        foot={
          <>
            <Pill tone={reviewLeft ? "warn" : "good"}>{reviewLeft ? "uncategorized" : "all clear"}</Pill>
            <span aria-hidden className="h-1 w-[84px] overflow-hidden rounded bg-track">
              <span className={cx("block h-full rounded transition-[width] duration-700", reviewLeft ? "bg-state-progress" : "bg-state-done")} style={{ width: `${categorized * 100}%` }} />
            </span>
          </>
        }
      >
        <CountUp value={reviewLeft} format={(n) => String(Math.round(n))} duration={500} />
      </Tile>
    </section>
  );
}

/** Money in and out per month; the switch shows what counting card payments would do. */
export function CashFlow({ data, range }: { data: FinanceOverview; range: number }) {
  const [transfers, setTransfers] = useState(false);
  const [hover, setHover] = useState<number | null>(null);
  const months = data.months.slice(-range);
  const bars = cashBars(months, transfers);
  const W = 560;
  const H = 190;
  const left = 36;
  const bottom = 22;
  const top = 8;
  const inner = H - top - bottom;
  const { top: max, step } = niceScale(Math.max(...bars.flatMap((b) => [b.inflow, b.outflow]), 0));
  const cw = (W - left) / bars.length;
  const bw = Math.min(18, cw * 0.3);
  const y = (v: number) => H - bottom - (inner * v) / max;
  const ticks = Array.from({ length: Math.round(max / step) + 1 }, (_, i) => i * step);
  const note = cashNote(months, transfers);
  const h = hover === null ? null : bars[hover];
  return (
    <Panel
      title="Cash flow"
      delay={240}
      aside={
        <>
          <span className="inline-flex items-center gap-[5px]">
            <i className="size-2 rounded-[2px]" style={{ background: GREEN }} />
            In
          </span>
          <span className="inline-flex items-center gap-[5px]">
            <i className="size-2 rounded-[2px]" style={{ background: RED }} />
            Out
          </span>
          <Toggle checked={transfers} onChange={setTransfers} label="Count card payments" />
        </>
      }
    >
      <svg viewBox={`0 0 ${W} ${H}`} className="block h-auto w-full" role="img" aria-label={`Money in and out over ${bars.length} months`} onMouseLeave={() => setHover(null)}>
        <defs>
          <pattern id="fin-hatch" width="5" height="5" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
            <rect width="5" height="5" fill={RED} opacity={0.55} />
            <rect width="2" height="5" fill={RED} />
          </pattern>
        </defs>
        {ticks.map((v) => (
          <g key={v}>
            <line x1={left} x2={W} y1={y(v)} y2={y(v)} stroke="var(--border-subtle)" />
            <text x={left - 6} y={y(v) + 3} textAnchor="end" fontSize={10} fill="var(--text-tertiary)">
              {axisMoney(v)}
            </text>
          </g>
        ))}
        {/* Re-keyed so switching range or transfers grows the bars again. */}
        <g key={`${range}-${transfers}`}>
          {bars.map((b, i) => {
            const cx0 = left + cw * i + cw / 2;
            const delay = { animationDelay: `${i * 45}ms` };
            return (
              <g key={b.month}>
                <rect className="fin-bar" style={delay} x={cx0 - bw - 1.5} y={y(b.inflow)} width={bw} height={H - bottom - y(b.inflow)} rx={2.5} fill={GREEN} />
                <rect className="fin-bar" style={delay} x={cx0 + 1.5} y={y(b.outflow)} width={bw} height={H - bottom - y(b.outflow)} rx={2.5} fill={RED} opacity={0.85} />
                {b.transfers > 0 && (
                  <rect className="fin-bar" style={{ animationDelay: `${i * 45 + 120}ms` }} x={cx0 + 1.5} y={y(b.outflow)} width={bw} height={(inner * b.transfers) / max} rx={2.5} fill="url(#fin-hatch)" />
                )}
                <text x={cx0} y={H - 6} textAnchor="middle" fontSize={10} fill="var(--text-tertiary)">
                  {monthLabel(b.month)}
                  {i === bars.length - 1 ? " ·MTD" : ""}
                </text>
                <rect x={left + cw * i} y={top} width={cw} height={inner} fill={hover === i ? "var(--hover)" : "transparent"} fillOpacity={0.5} onMouseEnter={() => setHover(i)} />
              </g>
            );
          })}
        </g>
      </svg>
      {h && hover !== null && (
        <div
          className="pointer-events-none absolute z-10 -translate-x-1/2 rounded-lg border border-border-strong bg-surface-raised px-[9px] py-[7px] text-12 shadow-[var(--shadow-float)]"
          // The chart spans the panel inside its 14px padding.
          style={{ left: `calc(14px + (100% - 28px) * ${(left + cw * hover + cw / 2) / W})`, top: 52 }}
        >
          <div className="font-semibold text-fg">
            {monthLabel(h.month, "long")}
            {hover === bars.length - 1 ? " (so far)" : ""}
          </div>
          <div>
            <span className="text-pill-done-fg">In {money(h.inflow)}</span> · <span className="text-pill-blocked-fg">Out {money(h.outflow)}</span>
          </div>
          <div className="text-fg-secondary">Net {money(h.inflow - h.outflow, { sign: true })}</div>
        </div>
      )}
      <p className={cx("mt-[6px] min-h-[18px] text-12 transition-colors", note.warn ? "text-pill-blocked-fg" : "text-fg-tertiary")}>
        {note.warn && "⚠ "}
        {note.text}
      </p>
    </Panel>
  );
}

/** Spending by category for the last full month; a row asks the assistant why. */
export function Spending({ data, onAsk }: { data: FinanceOverview; onAsk: (q: string) => void }) {
  const cats = data.categories;
  const max = Math.max(...cats.map((c) => c.spent_cents), 1);
  const month = monthLabel(data.category_month, "long");
  return (
    <Panel title={`Spending · ${month}`} aside="vs months before" delay={290}>
      {cats.length === 0 ? (
        <p className="py-6 text-center text-12 text-fg-tertiary">No spending in {month} yet.</p>
      ) : (
        <ul className="flex flex-col">
          {cats.map((c, i) => {
            const d = spendDelta(c.spent_cents, c.average_cents);
            const question =
              d.tone === "hot" ? `Why was ${c.category} up ${d.label} in ${month}?` : `What made up my ${c.category} spending in ${month}?`;
            return (
              <li key={c.category}>
                <button
                  type="button"
                  onClick={() => onAsk(question)}
                  title={question}
                  className="-mx-[6px] grid h-[26px] w-[calc(100%+12px)] grid-cols-[96px_1fr_64px_52px] items-center gap-2 rounded-md px-[6px] text-left hover:bg-row-hover"
                >
                  <span className="truncate text-13 text-fg-2">{c.category}</span>
                  <span className="h-[6px] overflow-hidden rounded-full bg-track">
                    <span
                      className="fin-fill block h-full rounded-full"
                      style={{ width: `${(c.spent_cents / max) * 100}%`, background: d.tone === "hot" ? "var(--pill-blocked-fg)" : ACCENT, animationDelay: `${350 + i * 50}ms` }}
                    />
                  </span>
                  <span className="text-right text-13 tabular-nums">{money(c.spent_cents)}</span>
                  <span className={cx("text-right text-11", d.tone === "hot" ? "text-pill-blocked-fg" : d.tone === "cool" ? "text-pill-done-fg" : "text-fg-tertiary")}>{d.label}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </Panel>
  );
}

/** Transactions waiting for a category: a pick becomes a rule for that merchant. */
export function Review({
  data,
  leaving,
  saveRules,
  onSaveRules,
  onPick,
}: {
  data: FinanceOverview;
  leaving: Set<string>;
  saveRules: boolean;
  onSaveRules: (v: boolean) => void;
  onPick: (item: FinanceReviewItem, category: string) => void;
}) {
  const rows = data.review;
  const done = rows.every((r) => leaving.has(r.id));
  return (
    <Panel title="Needs review" aside={<Toggle checked={saveRules} onChange={onSaveRules} label="Save picks as rules" />} delay={340}>
      <ul aria-label="Transactions to categorize">
        {rows.map((r) => (
          <li key={r.id} className="fin-row border-t border-border-subtle first:border-t-0" data-leaving={leaving.has(r.id)}>
            <div className="grid grid-cols-[1fr_auto] gap-x-[10px] gap-y-1 py-[9px]">
              <div className="min-w-0">
                <div className="truncate font-mono text-12 text-fg-2">{r.description}</div>
                <div className="text-11 text-fg-tertiary">
                  {dayLabel(r.posted_on)} · {accountLabel(r.account_name, r.account_mask)}
                </div>
              </div>
              <div className={cx("text-right tabular-nums text-13", r.amount_cents > 0 ? "text-pill-done-fg" : "text-fg-2")}>{money(r.amount_cents, { cents: true, sign: true })}</div>
              <div className="col-span-2 flex flex-wrap items-center gap-[6px]">
                {r.suggestions.map((c, j) => (
                  <button
                    key={c}
                    type="button"
                    disabled={leaving.has(r.id)}
                    onClick={() => onPick(r, c)}
                    className={cx(
                      "h-6 rounded-full border px-[9px] text-12 transition-colors",
                      j === 0 ? "border-accent/40 bg-accent-soft text-fg hover:border-accent" : "border-border-strong text-fg-secondary hover:bg-hover hover:text-fg",
                    )}
                  >
                    {j === 0 && <span aria-hidden className="mr-1 text-10 text-accent">✦</span>}
                    {c}
                  </button>
                ))}
                <select
                  aria-label={`Another category for ${r.description}`}
                  value=""
                  disabled={leaving.has(r.id)}
                  onChange={(e) => e.target.value && onPick(r, e.target.value)}
                  className="h-6 rounded-full border border-border-strong bg-transparent px-2 text-12 text-fg-tertiary hover:text-fg"
                >
                  <option value="">Other…</option>
                  {data.categories_all
                    .filter((c) => !r.suggestions.includes(c))
                    .map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                </select>
              </div>
            </div>
          </li>
        ))}
      </ul>
      {done && (
        <p className="motion-rise py-[18px] text-center text-12 text-fg-tertiary">
          {data.review_count > rows.length ? "Loading the next ones…" : "✓ All caught up. Rules will handle these merchants next time."}
        </p>
      )}
    </Panel>
  );
}

const AVATAR = ["var(--agent-atlas)", "var(--agent-quill)", "var(--agent-sentry)", "var(--agent-juno)", "var(--agent-nova)", "var(--agent-relay)", "var(--agent-pike)", "var(--agent-echo)"];

function hue(label: string) {
  let h = 0;
  for (const ch of label) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return AVATAR[h % AVATAR.length];
}

/** Monthly charges: price changes and new ones stand out. */
export function Recurring({ items }: { items: Schemas["FinanceRecurring"][] }) {
  const total = items.reduce((s, r) => s + r.amount_cents, 0);
  return (
    <Panel title="Recurring" aside={items.length ? `${money(total, { cents: true })}/mo` : undefined} delay={390}>
      {items.length === 0 ? (
        <p className="py-6 text-center text-12 text-fg-tertiary">No monthly charges found yet. They show up after two months of statements.</p>
      ) : (
        <ul>
          {items.map((r, i) => (
            <li key={r.label} className="motion-rise grid grid-cols-[22px_1fr_auto] items-center gap-[10px] border-t border-border-subtle py-2 first:border-t-0" style={{ animationDelay: `${450 + i * 60}ms` }}>
              <span aria-hidden className="grid size-[22px] place-items-center rounded-md text-11 font-semibold text-on-accent" style={{ background: hue(r.label) }}>
                {r.label.charAt(0)}
              </span>
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-[6px]">
                  <span className="truncate text-13 text-fg">{r.label}</span>
                  {r.previous_cents !== undefined && r.changed_on && (
                    <span className="flex-none rounded-full bg-pill-human-bg px-[6px] py-px text-10 font-semibold text-pill-human-fg">
                      {r.amount_cents > r.previous_cents ? "↑" : "↓"} {money(Math.abs(r.amount_cents - r.previous_cents), { cents: true })} since {monthLabel(r.changed_on)}
                    </span>
                  )}
                  {r.new && <span className="flex-none rounded-full bg-accent-soft px-[6px] py-px text-10 font-semibold text-accent">New</span>}
                </div>
                <div className="text-11 text-fg-tertiary">Monthly · around the {ordinal(r.day_of_month)}</div>
              </div>
              <span className="tabular-nums text-13">{money(r.amount_cents, { cents: true })}</span>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
