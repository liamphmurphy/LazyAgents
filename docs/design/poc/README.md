# Designs proof of concept (LAZYAGENTS-145)

Throwaway code that tests the riskiest assumptions in `../ADR-designs.md` §5. It isn't production code. Later tickets port it into `web/lib/designs/`.

| File | What it proves |
|---|---|
| `validate.mjs` | Validates schema 1: `../schema/design-1.schema.json` plus the semantic rules |
| `tokens.mjs` | Parses `web/styles/tokens.css` into a token snapshot, with the hash the sync badge compares |
| `metrics.mjs` | Builds the Inter advance-width tables, so text measures the same in the browser and in Node |
| `layout.mjs` | The shared display list: auto layout, the text model, instances, images, connectors, stickies |
| `svg.mjs` | Draws the display list as SVG, the editor renderer's output |
| `parity.mjs` | Renders a server PNG with resvg and an editor PNG with Chromium, then diffs them with pixelmatch |
| `large.mjs`, `panzoom.mjs` | Pan/zoom timing for a ~2k-node design, with a Chromium trace |
| `mermaid.mjs` | Turns a Mermaid flowchart into nodes and connectors, laid out with ELK |
| `run.mjs` | Runs everything and writes `evidence/` and `evidence/report.json` |

```sh
npm ci
python3 -I fetch-fonts.py      # Inter 4.1 (OFL), checksum-pinned, into fonts/ (git-ignored)
node run.mjs                   # needs a Playwright Chromium (PLAYWRIGHT_BROWSERS_PATH or npx playwright-core install chromium)
node validate.mjs ../fixtures/acceptance-design.json
```

Inter is © The Inter Project Authors and licensed under the SIL Open Font License 1.1 (`fonts/LICENSE.txt`).
