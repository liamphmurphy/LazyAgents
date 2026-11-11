# Acceptance deck: all four outputs (LAZYAGENTS-124)

`run.sh` renders the whole acceptance deck (`../fixtures/acceptance-deck.json`, 18 slides) with the app's own code (`web/lib/slides`) three ways:

- the editor, thumbnail and presentation SVG, drawn by Chromium at 1920 × 1080 with the slide fonts;
- the PDF export;
- the PPTX export.

It then opens the PPTX in LibreOffice in a container and checks everything with `check.py`:

1. LibreOffice converts the PPTX to PDF (exit 0, 18 pages) and round-trips it to PPTX and back to PDF (exit 0, 18 pages).
2. The PPTX's structure:
   - 12 192 000 × 6 858 000 EMU, one slide per shown slide, no `normAutofit`;
   - every element is a native shape named by its id, placed in its frame and never a picture of itself, except images, which are `p:pic`;
   - groups are `p:grpSp` holding their children;
   - shapes keep their `prstGeom` preset;
   - gradients are `a:gradFill`;
   - fill, outline and image transparency, dashes and arrowheads are written;
   - text stays in editable text frames, with justify (`algn="just"`), exact line spacing (`a:spcPts`), strikethrough and highlight.
3. The pixels: the editor and the PDF differ on less than 2% of pixels on every slide.

```sh
docs/slides/acceptance/run.sh                    # the acceptance deck → evidence/
docs/slides/acceptance/run.sh deck.json out/     # any deck, e.g. a v1 one
```

It needs node (with `web/` installed and Playwright's Chromium) and docker. It exits 1 on any failed check.

## Evidence (LibreOffice 7.4.7.2)

**Result: 0 failures.**

- `evidence/report.json`:
  - the LibreOffice version and conversion results;
  - the PPTX counts: 83 named elements, 2 groups, 19 non-basic presets, 3 gradients, 3 transparent elements, 5 dashed;
  - per-slide pixel differences.
- `evidence/compare-NN.png`: per slide, the editor, the app's PDF and LibreOffice's render of the PPTX, side by side.
- `evidence/deck.pdf`, `deck.pptx`, `deck-libreoffice.pdf`: the outputs themselves.

**Editor vs PDF:** 0.1–1.4% of pixels differ by more than a quarter, on anti-aliasing at glyph and shape edges.

**PDF vs LibreOffice:** 0.7–2.4% of pixels differ on most slides. Slides 2 and 7 differ on about 4%, from three LibreOffice rendering habits:
- LibreOffice puts bullets a little higher;
- it adds about 4 px of first-line leading under exact line spacing;
- it draws one strikethrough across a run's trailing space.

Line breaks, frames, colours, gradients, dashes and groups match.

**Not checked here:**
- **PowerPoint.** The sandbox has none. Someone opens `evidence/deck.pptx` in PowerPoint and checks that there's no repair prompt and that a title and a shape's text can be edited (ADR §5).
- **Presentation mode.** It draws the same `SlideSvg` as the editor, so the editor column stands for it. `web/e2e/slides-*.spec.ts` covers presenting.

## Found and fixed here

Cropped or covered images were placed at the wrong size in the PPTX:
- an image narrower than its frame was squashed into a narrow box;
- a wider one left blank space below it.

pptxgenjs makes the crop box the picture's size, so the export now gives it the image at the scale that covers the frame, and a box exactly the frame's size. The v1 deck's image slide is the only v1 output this changes: its picture's width goes from 3 225 397 to 5 080 000 EMU, the frame's width. Every other part of the v1 PPTX is byte-identical to `main`'s, and its PDF pages and editor renders are pixel-identical.
