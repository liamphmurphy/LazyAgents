# Slides export proof of concept (LAZYAGENTS-103)

Throwaway code that backs [the Slides ADR](../ADR-slides.md). It isn't part of the app and isn't built by CI.

`layout.mjs` turns a deck slide into a display list in 1920 × 1080 slide units. `run.mjs` renders `../fixtures/acceptance-deck.json` three ways from that one layout:

- `out/slide-N.svg`: what the editor and the presentation draw;
- `out/deck.pdf`: jsPDF, one 960 × 540 pt page per slide, fonts embedded;
- `out/deck.pptx`: pptxgenjs, native editable text boxes and shapes.

`verify.py` checks the outputs with PyMuPDF and python-pptx and writes `evidence/`.

```sh
cd docs/slides/poc
npm ci && node run.mjs
python3 -m venv /tmp/venv && /tmp/venv/bin/pip install pymupdf python-pptx
/tmp/venv/bin/python -I verify.py "$PWD/out" "$PWD/evidence"   # exits 1 on any failed check
```

Fonts are read from `/usr/share/fonts/truetype/liberation` (Debian's `fonts-liberation`). The app will bundle the upstream Liberation 2.x release instead (see ADR §3.4).
