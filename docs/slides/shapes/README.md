# Preset shapes (LAZYAGENTS-117)

Besides `rect`, `roundRect` and `ellipse`, a shape element can be any of these
PowerPoint `prstGeom` presets. The name is stored as is and written straight
into the PPTX, so the shape stays native and editable there.

| Category | Shapes |
|---|---|
| Basics | `triangle`, `rtTriangle`, `diamond`, `pentagon`, `hexagon`, `octagon` |
| Arrows & flow | `rightArrow`, `leftArrow`, `upArrow`, `downArrow`, `chevron`, `homePlate` |
| Callouts | `wedgeRectCallout`, `wedgeRoundRectCallout` |
| Accents | `star5`, `plus`, `donut`, `heart` |

`web/lib/slides/geometry.ts` draws them for the editor, presentation,
thumbnails and PDF from PowerPoint's `presetShapeDefinitions.xml` formulas at
their default adjust values, and gives each its text rectangle (the inner box
of a triangle, star or callout), so text wraps where PowerPoint wraps it. The
server's list is `Shapes` in `server/internal/deck/deck.go`; a web test fails
if the two drift. The v1 shapes keep their own drawing and full-frame text box,
so v1 decks render unchanged.

Two presets reach past their frame, as in PowerPoint: the callouts' tail
points 12.5% of the height below it, and the heart bulges 0.35% past its sides.

## Evidence

`compare.sh` builds the acceptance deck's `s_preset_shapes` slide with the
app's own export code, converts the PPTX to PDF with LibreOffice (in a Docker
container) and compares that with the app's PDF shape by shape. Every shape's
outline agrees (IoU ≥ 0.99; the check fails under 0.95):

- `evidence/report.json`: LibreOffice version and per-shape IoU
- `evidence/shapes-app-vs-libreoffice.png`: app PDF (top) over LibreOffice's render of the PPTX (bottom), with text
- `evidence/shapes-diff.png`, `evidence/shapes-bare-*.png`: pixel differences, with and without text
- `evidence/shapes.pptx`, `shapes.pdf`, `shapes.svg`: the outputs themselves

```sh
docs/slides/shapes/compare.sh   # needs node (web/ installed) and docker
```
