// npm run gen:inter-metrics: rewrites lib/designs/inter-metrics.json from the
// Inter TTFs in public/fonts/designs. lib/designs-measure.test.mjs fails when
// the two drift.
import { readFileSync, writeFileSync } from "node:fs";
import { buildMetrics } from "../lib/designs/inter-metrics-gen.ts";

const dir = new URL("../public/fonts/designs/", import.meta.url);
const metrics = buildMetrics((f) => readFileSync(new URL(f, dir)));
writeFileSync(new URL("../lib/designs/inter-metrics.json", import.meta.url), JSON.stringify(metrics) + "\n");
console.log("wrote lib/designs/inter-metrics.json");
