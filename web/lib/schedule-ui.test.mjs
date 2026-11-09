import test from "node:test";
import assert from "node:assert/strict";
import { clockLabel, describeSchedule, moneyLabel, tasksNeedingYou } from "./schedule-ui.ts";

const base = { weekdays: [], month_day: 1, at_time: "08:00", cron: "" };

test("schedules read as a person would say them", () => {
  assert.equal(clockLabel("00:05"), "12:05 AM");
  assert.equal(clockLabel("17:30"), "5:30 PM");
  assert.equal(describeSchedule({ ...base, frequency: "daily" }), "Every day at 8:00 AM");
  assert.equal(describeSchedule({ ...base, frequency: "weekly", weekdays: [1] }), "Every Monday at 8:00 AM");
  assert.equal(describeSchedule({ ...base, frequency: "weekly", weekdays: [5, 1, 2, 3, 4] }), "Weekdays at 8:00 AM");
  assert.equal(describeSchedule({ ...base, frequency: "monthly", month_day: 22 }), "22nd of the month at 8:00 AM");
  assert.equal(describeSchedule({ ...base, frequency: "monthly", month_day: 11 }), "11th of the month at 8:00 AM");
  assert.equal(describeSchedule({ ...base, frequency: "cron", cron: "0 9 * * 1" }), "Cron: 0 9 * * 1");
});

test("only a waiting latest run counts for the badge", () => {
  assert.equal(tasksNeedingYou([{ enabled: true, runs: [{ state: "waiting" }] }, { enabled: true, runs: [{ state: "done" }, { state: "waiting" }] }, { enabled: true, runs: [] }]), 1);
  assert.equal(tasksNeedingYou(undefined), 0);
  assert.equal(moneyLabel(1_000_000), "$1.00");
});
