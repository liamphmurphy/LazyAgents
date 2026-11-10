/**
 * The Finance app's pure helpers: money formatting, chart scales and the
 * comparisons the dashboard draws. Amounts are integer cents; negative is money out.
 */

const whole = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
const exact = new Intl.NumberFormat("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** $1,234 (or $1,234.56 with cents); a real minus sign, and a plus when `sign` asks for one. */
export function money(cents: number, opts: { cents?: boolean; sign?: boolean } = {}): string {
  const abs = Math.abs(cents) / 100;
  const body = "$" + (opts.cents ? exact.format(abs) : whole.format(Math.round(abs)));
  if (cents < 0 && (opts.cents ? abs > 0 : Math.round(abs) > 0)) return "−" + body;
  return opts.sign && cents > 0 ? "+" + body : body;
}

/** Axis labels: 0, 2k, 1.5k, 12k. */
export function axisMoney(cents: number): string {
  const d = cents / 100;
  if (Math.abs(d) < 1000) return whole.format(d);
  const k = d / 1000;
  return `${Number.isInteger(k) ? k : k.toFixed(1)}k`;
}

/** A round top for a chart's axis and its gridline step, at least `max`. */
export function niceScale(max: number, lines = 4): { top: number; step: number } {
  if (max <= 0) return { top: lines * 100_00, step: 100_00 };
  const raw = max / lines;
  const pow = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * pow).find((s) => s >= raw) ?? 10 * pow;
  return { top: Math.ceil(max / step) * step, step };
}

/** "2026-10-01" → "Oct". UTC, so a date never slips into the month before. */
export function monthLabel(iso: string, style: "short" | "long" = "short"): string {
  return new Date(`${iso.slice(0, 10)}T00:00:00Z`).toLocaleString("en-US", { month: style, timeZone: "UTC" });
}

/** "2026-11-05" → "Nov 5". */
export function dayLabel(iso: string): string {
  return new Date(`${iso.slice(0, 10)}T00:00:00Z`).toLocaleString("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
}

export type Delta = { label: string; tone: "hot" | "cool" | "flat" | "new" };

/** Spending against its average: +15% or more is worth a look, under −5% is good news. */
export function spendDelta(spent: number, average: number): Delta {
  if (average <= 0) return { label: "new", tone: "new" };
  const pct = Math.round(((spent - average) / average) * 100);
  if (Math.abs(pct) < 5) return { label: "≈ avg", tone: "flat" };
  return { label: `${pct > 0 ? "+" : ""}${pct}%`, tone: pct > 15 ? "hot" : pct < -5 ? "cool" : "flat" };
}

/** An SVG path through values scaled into w×h, with a little headroom. Empty for fewer than two points. */
export function sparkPath(values: number[], w: number, h: number): string {
  if (values.length < 2) return "";
  const lo = Math.min(...values);
  const hi = Math.max(...values);
  const span = hi - lo || 1;
  return values
    .map((v, i) => {
      const x = (i * w) / (values.length - 1);
      const y = h - 2 - ((v - lo) / span) * (h - 4);
      return `${i ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join("");
}

export type CashMonth = { month: string; inflow_cents: number; outflow_cents: number; transfer_out_cents: number; net_worth_cents?: number };

/** What the cash flow chart draws for a month: out includes transfers only when the person counts them. */
export function cashBars(months: CashMonth[], countTransfers: boolean) {
  return months.map((m) => ({
    month: m.month,
    inflow: m.inflow_cents,
    outflow: m.outflow_cents + (countTransfers ? m.transfer_out_cents : 0),
    transfers: countTransfers ? m.transfer_out_cents : 0,
  }));
}

/** The note under the chart: what transfers would do to spending, or the period's net. */
export function cashNote(months: CashMonth[], countTransfers: boolean): { text: string; warn: boolean } {
  const transfers = months.reduce((s, m) => s + m.transfer_out_cents, 0);
  if (countTransfers) {
    return { text: `Spending overstated by ${money(transfers)}: card payments counted once as purchases and again as the payment.`, warn: true };
  }
  const net = months.reduce((s, m) => s + m.inflow_cents - m.outflow_cents, 0);
  const tail = transfers > 0 ? ` · ${money(transfers)} in transfers between your accounts left out` : "";
  return { text: `Net ${money(net, { sign: true })} over ${months.length} ${months.length === 1 ? "month" : "months"}${tail}.`, warn: false };
}

/** Net worth points for a sparkline: the months that have one. */
export function netWorthSeries(months: CashMonth[]): number[] {
  return months.flatMap((m) => (m.net_worth_cents === undefined ? [] : [m.net_worth_cents]));
}

/** Monthly savings rate (0–1) for months with income. */
export function savingsSeries(months: CashMonth[]): number[] {
  return months.flatMap((m) => (m.inflow_cents > 0 ? [(m.inflow_cents - m.outflow_cents) / m.inflow_cents] : []));
}

/** Readable form of a bank description for a review row's account: "Visa ••0921". */
export function accountLabel(name: string, mask: string): string {
  return mask && !name.includes(mask) ? `${name} ••${mask}` : name;
}

/** "1st", "22nd". */
export function ordinal(n: number): string {
  const s = n % 100 >= 11 && n % 100 <= 13 ? "th" : ({ 1: "st", 2: "nd", 3: "rd" } as Record<number, string>)[n % 10] ?? "th";
  return `${n}${s}`;
}

/** What a person types for a balance, "-1,234.56" or "$1,234", in cents; null when it isn't an amount. */
export function parseDollars(s: string): number | null {
  const t = s.replace(/[$,\s]/g, "").replace("−", "-");
  if (!/^-?\d+(\.\d{0,2})?$/.test(t)) return null;
  return Math.round(Number(t) * 100);
}
