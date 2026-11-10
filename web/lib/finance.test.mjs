import { test } from "node:test";
import assert from "node:assert/strict";
import { parseDollars, accountLabel, axisMoney, cashBars, cashNote, dayLabel, money, monthLabel, netWorthSeries, niceScale, ordinal, savingsSeries, sparkPath, spendDelta } from "./finance.ts";

test("money formats cents with a real minus and an optional plus", () => {
  assert.equal(money(123456), "$1,235");
  assert.equal(money(123456, { cents: true }), "$1,234.56");
  assert.equal(money(-250, { cents: true }), "−$2.50");
  assert.equal(money(194000, { sign: true }), "+$1,940");
  assert.equal(money(-40, {}), "$0"); // rounds to zero: no minus on $0
  assert.equal(money(0, { sign: true }), "$0");
});

test("axis labels and scales stay round", () => {
  assert.equal(axisMoney(0), "0");
  assert.equal(axisMoney(200000), "2k");
  assert.equal(axisMoney(150000), "1.5k");
  assert.deepEqual(niceScale(730000), { top: 800000, step: 200000 });
  assert.deepEqual(niceScale(0), { top: 40000, step: 10000 });
  const { top, step } = niceScale(523400);
  assert.ok(top >= 523400 && top / step <= 5);
});

test("dates read in UTC", () => {
  assert.equal(monthLabel("2026-10-01"), "Oct");
  assert.equal(monthLabel("2026-01-01", "long"), "January");
  assert.equal(dayLabel("2026-11-05"), "Nov 5");
});

test("spendDelta flags what's well above average", () => {
  assert.deepEqual(spendDelta(98000, 46000), { label: "+113%", tone: "hot" });
  assert.deepEqual(spendDelta(61000, 59000), { label: "≈ avg", tone: "flat" });
  assert.deepEqual(spendDelta(19000, 20000), { label: "-5%", tone: "flat" });
  assert.deepEqual(spendDelta(15000, 20000), { label: "-25%", tone: "cool" });
  assert.deepEqual(spendDelta(11000, 10000), { label: "+10%", tone: "flat" });
  assert.deepEqual(spendDelta(5000, 0), { label: "new", tone: "new" });
});

test("sparkPath spans the box", () => {
  assert.equal(sparkPath([1], 84, 26), "");
  const d = sparkPath([1, 3, 2], 84, 26);
  assert.match(d, /^M0\.0,24\.0L42\.0,2\.0L84\.0,13\.0$/);
  assert.match(sparkPath([5, 5], 10, 10), /^M0\.0,8\.0L10\.0,8\.0$/);
});

const months = [
  { month: "2026-10-01", inflow_cents: 680000, outflow_cents: 524000, transfer_out_cents: 190000, net_worth_cents: 8_248_000 },
  { month: "2026-11-01", inflow_cents: 455000, outflow_cents: 261000, transfer_out_cents: 90000 },
];

test("cash flow counts transfers only when asked", () => {
  assert.deepEqual(cashBars(months, false)[0], { month: "2026-10-01", inflow: 680000, outflow: 524000, transfers: 0 });
  assert.deepEqual(cashBars(months, true)[0], { month: "2026-10-01", inflow: 680000, outflow: 714000, transfers: 190000 });
  assert.deepEqual(cashNote(months, true), { text: "Spending overstated by $2,800: card payments counted once as purchases and again as the payment.", warn: true });
  assert.deepEqual(cashNote(months, false), { text: "Net +$3,500 over 2 months · $2,800 in transfers between your accounts left out.", warn: false });
  assert.equal(cashNote([{ month: "2026-11-01", inflow_cents: 0, outflow_cents: 100, transfer_out_cents: 0 }], false).text, "Net −$1 over 1 month.");
});

test("series skip months without data", () => {
  assert.deepEqual(netWorthSeries(months), [8_248_000]);
  assert.equal(savingsSeries(months).length, 2);
  assert.equal(savingsSeries([{ month: "x", inflow_cents: 0, outflow_cents: 5, transfer_out_cents: 0 }]).length, 0);
});

test("small labels", () => {
  assert.equal(accountLabel("Visa", "0921"), "Visa ••0921");
  assert.equal(accountLabel("Checking ••4417", "4417"), "Checking ••4417");
  assert.equal(accountLabel("Cash", ""), "Cash");
  assert.deepEqual([1, 2, 3, 4, 11, 12, 13, 21, 22, 31].map(ordinal), ["1st", "2nd", "3rd", "4th", "11th", "12th", "13th", "21st", "22nd", "31st"]);
});

test("parseDollars reads what people type", () => {
  assert.equal(parseDollars("-1,234.56"), -123456);
  assert.equal(parseDollars("$1,234"), 123400);
  assert.equal(parseDollars("−454.6"), -45460);
  assert.equal(parseDollars("0.07"), 7);
  assert.equal(parseDollars("12.345"), null);
  assert.equal(parseDollars("abc"), null);
  assert.equal(parseDollars(""), null);
});
