# ADR: Slides deck model, persistence, persona review and export

- **Status:** Proposed (needs Liam's approval before any Slides implementation ticket starts)
- **Ticket:** LAZYAGENTS-103 (first ticket of the epic "Slides app: create, present, and export slide decks")
- **Evidence:** `docs/slides/poc/` (proof of concept), `docs/slides/fixtures/acceptance-deck.json` (shared acceptance deck), `docs/slides/poc/evidence/` (PDF, PPTX, page renders, verification report)

## 1. Context

Slides should work like Docs. The person edits directly. A persona chat sits beside each deck, with its own conversation. Work autosaves into versions. The persona's edits wait for the person to review them. An Export menu offers the outputs. Unlike Docs, Slides must also produce a browser presentation, a PDF and a PPTX that all look like the editor. The PPTX must keep text and shapes editable.

What Docs already gives us, and how it works today:

| Docs piece | Where | What it does |
|---|---|---|
| Document = text canvas | `server/internal/canvas`, `chat_canvases` + `chat_canvas_versions` (`kind IN ('text','svg')`) | Every save is an immutable version (`seq`). `Save`/`Patch` take a `base` seq and return 409 when it's stale. `Restore` brings back an old version. |
| One chat per document | `server/internal/docs` (`chat_spaces.app = 'docs'`, `SpaceInstructions`) | The document's chat sits in an app space, so it stays out of the chat list. The space instructions tell the persona to `canvas_read`, then `canvas_patch`. |
| Opening outside the chat | `GET /canvases`, `GET /canvases/{id}` | Ownership is checked against workspace and user. Docs filters the list to `kind === "text"`. |
| Persona review | `web/lib/doc-review.ts`, `POST /docs/{id}/reviewed`, `chat_canvases.reviewed_seq` | The review base is the person's last own version or the last reviewed seq, whichever is newer. The diff between that base and head is split into decisions. The person accepts or rejects each one, and the result is saved as a user version with a note. |
| Live edits and conflicts | `doc-screen.tsx` | The page is read-only while the persona is editing. A 409 shows "This document changed while you were typing", with **Use the latest version** and **Keep mine**. |
| Export | `export-menu.tsx` | DOCX and Markdown are built client-side (lazily loaded `docx`). "PDF" opens a print page, and the person then uses the browser's print dialog. |

The deck needs:
- 2D geometry.
- Rich text runs.
- Images with a crop.
- Basic shapes.
- Themes.
- Stable identities, so that review, merging and undo can work per slide or element instead of per line.

## 2. Content model options

| Option | For | Against | Verdict |
|---|---|---|---|
| **A. Own structured JSON deck schema** (slides → elements with frames, paragraphs/runs, theme tokens) stored as a new canvas kind | Maps 1:1 to PPTX shapes and to an SVG/PDF display list. Stable IDs make review, merge and undo per element easy. The server can validate it. Fits the existing canvas version and conflict machinery. | We own a schema, its validator and its migrations. | **Chosen** |
| B. SVG per slide (reuse the `svg` canvas kind) | Already versioned. The persona already edits SVG with `data-vid` ops. | SVG has no text wrapping and no paragraph model. Turning arbitrary SVG into editable PPTX text is lossy, and themes and layouts can't be expressed. | Rejected |
| C. Markdown slides (Marp/reveal style; reuse the text canvas and `doc-review.ts` unchanged) | Cheapest. Persona edits are trivial. Docs review is reused as is. | There's no free positioning, image crop or shapes, and PPTX layouts would be guessed. It fails the editor criteria (epic criterion 3). | Rejected |
| D. OOXML/PPTX as the canonical format | Exact for PowerPoint. | Huge surface. Hard to diff, validate or have a persona edit. It ties the browser renderer to a PPTX engine. | Rejected |
| E. Third-party editor engine (tldraw: source-available with a commercial licence; Polotno: commercial; Excalidraw: MIT, but whiteboard semantics; Konva/Fabric: MIT canvas libraries) | Ready-made selection and transform handles. | We would still need our own schema for export and review. Canvas-drawn text hurts accessibility and makes in-place text editing harder. Licence or product fit is wrong for the first three. | Rejected. Selection, drag and resize handles are built in React over the SVG slide (a small, known surface). |

## 3. Decision

### 3.1 Canonical deck schema (`schema: 1`)

The deck is one JSON document, stored as the body of a canvas with the new kind `deck`. A complete example is `docs/slides/fixtures/acceptance-deck.json`.

```
Deck      { schema: 1, size: {w:1920,h:1080}, theme: Theme, assets: {[assetId]: AssetRef}, slides: Slide[] }
Theme     { name, fonts: {heading: Slot, body: Slot}, colors: {background,text,heading,accent,accent2,muted,surface: "#RRGGBB"} }
Slot      "sans" | "serif" | "mono"
Slide     { id, layout: Layout, background?: {color: Paint}, notes?: string, hidden?: false, elements: Element[] }   // paint order = array order
Layout    "title" | "title-body" | "two-column" | "image-right" | "section" | "blank"
Element   common { id, type, frame: {x,y,w,h}, rotation?: deg, placeholder?: "title"|"subtitle"|"body"|"body2"|"image" }
  text    { paragraphs: Paragraph[], valign?: "top"|"middle"|"bottom" }
  shape   { shape: "rect"|"roundRect"|"ellipse"|<18 PowerPoint presets, see shapes/README.md>, fill?: Paint, fillOpacity?, stroke?, strokeWidth?, strokeOpacity?, dash?: Dash, radius?, text?: {paragraphs, valign} }
  line    { stroke, strokeWidth, strokeOpacity?, dash?: Dash, endArrow?: bool, startArrow?: bool }   // frame gives the two end points
  image   { asset: assetId, crop?: {l,t,r,b} (fractions of the source), alt: string, opacity? }   // always "cover" inside the frame after the crop
Paragraph { runs: Run[], align?: "left"|"center"|"right", bullet?: "disc"|"number", level?: 0..4, spaceBefore?: units }
Run       { text, size: units, bold?, italic?, underline?, color?: ColorRef, font?: "heading"|"body"|Slot }
ColorRef  theme token name | "#RRGGBB"
Paint     ColorRef | { type: "linear", angle: 0..360 (deg, clockwise from left→right), stops: [ColorRef, ColorRef] }   // LAZYAGENTS-118
Dash      "solid" | "dash" | "dot" | "dashDot" | "longDash"   // ×strokeWidth: 4·3, 1·1, 4·3·1·3, 8·3 (PowerPoint's dash, sysDot, dashDot, lgDash)
*Opacity  0..1, absent = 1   // PPTX a:alpha on the colour, a:alphaModFix on a picture; PDF ExtGState ca/CA; gradients are a PDF axial shading and PPTX a:gradFill
AssetRef  { mime: "image/png"|"image/jpeg", w, h, alt }   // pixel size; the bytes live in the blob store (§3.5)
```

**Validation and normalisation.** A Go package, `server/internal/slides`, validates and normalises every deck body. `canvas.normalize` calls it for kind `deck`, and it follows the pattern "Validate overrides in normalize, cap in one place". It enforces:
- unique IDs;
- known types and enums;
- finite numbers rounded to 0.01;
- colours that are tokens or `#RRGGBB`;
- every `asset` present in `assets`, and every `assets` key owned by this deck;
- caps: 200 slides, 100 elements per slide, 5 000 characters per text element, 512 KB body (the existing canvas cap).

The web app mirrors the types in `web/lib/slides/schema.ts`, so the editor never sends something the server would refuse.

**Schema versioning.** `schema` is an integer. Upgrades are pure functions `vN → vN+1` in Go, applied when a deck is read and persisted on its next save. Readers refuse a `schema` newer than they know, so old clients don't silently drop fields.

### 3.2 Stable IDs

- Slide and element IDs are opaque strings: a prefix plus 10 random base32 characters (`s_…`, `e_…`).
- The editor generates them. When a persona op omits one, the server assigns it.
- IDs are never reused within a deck. Duplicating a slide or a deck mints new IDs.
- IDs are the unit of diffing (review), three-way merge (conflicts), undo, persona ops and future comment anchors (like `data-vid` in SVG canvases).
- The PPTX export writes the element ID as the shape name. That made the PoC verifiable, and it will help with a future import.

### 3.3 Coordinates and aspect ratio

- Logical slide units: 1920 × 1080, 16:9 only in v1. The `size` field exists so that 4:3 can be added without a schema bump.
- The origin is top-left, y points down, and rotation is clockwise in degrees about the frame centre.
- One mapping, used everywhere:
  - **PDF:** points = units × 0.5, so a 960 × 540 pt page (13.333 × 7.5 in).
  - **PPTX:** EMU = units × 6350, so 12 192 000 × 6 858 000 (PowerPoint's "Widescreen" `LAYOUT_WIDE`).
  - **Font sizes:** given in units. pt = units × 0.5, so 48 units is 24 pt.
  - **Screen:** CSS px = units × (rendered width / 1920).

### 3.4 Text, layout and theme

- **Text** is paragraphs of styled runs:
  - run marks: bold, italic, underline, colour, size and font slot;
  - paragraph properties: alignment, disc or number bullets, five levels, and space before.
  - There's no autofit or shrink-to-fit anywhere. **Overflow is reported, never hidden.**
    - The editor outlines the box and shows a "Text doesn't fit" badge.
    - Present mode and PDF let the text spill, as PowerPoint does.
    - The export preflight lists the slides that overflow.
- **Shared text layout constants:**
  - 16-unit inset on all sides (8 pt);
  - line height = 1.2 × the paragraph's largest run, written to PPTX as *exact* line spacing (`a:spcPts`);
  - a 60-unit hanging indent per bullet level (PPTX `marL = 60 × (level+1)`, `indent = -60`).
  - Wrapping is greedy by words and measured with the same font files in the editor (canvas `measureText` against the loaded `FontFace`, with `font-kerning: none`) and in export.
- **Fonts are slots, not free families.** Each slot pairs a face we ship and embed with a metric-compatible face that PPTX names, so PowerPoint breaks lines where we did. Ship the upstream Liberation 2.x release, which is SIL OFL 1.1 and free to bundle and embed. Don't copy the fonts from a Linux system: Debian's `fonts-liberation` is 1.07, licensed GPLv2 with a font exception. The PoC used those system 1.07 files, which have the same metrics as 2.x. The table:

  | Slot | Shipped and embedded in PDF: Liberation Fonts **2.x** (SIL OFL 1.1) | Named in PPTX |
  |---|---|---|
  | sans | Liberation Sans | Arial |
  | serif | Liberation Serif | Times New Roman |
  | mono | Liberation Mono | Courier New |

- **Layouts** are templates applied when a slide is created or its layout changes. They create placeholder elements with explicit frames. Changing a slide's layout re-frames elements by placeholder role and leaves other elements alone. Rendering never inherits geometry from a layout, so four renderers can't disagree about inheritance.
- **Themes** are deck-level font slots and colour tokens. Elements refer to tokens (`"accent"`) or literal hex values. Switching theme recolours everything that uses tokens. v1 ships about four built-in themes, including light and dark. There's no custom theme editor beyond picking token colours.

### 3.5 Assets

- **Upload:** `POST /slides/{canvasId}/assets` (multipart).
  - Only PNG and JPEG. The editor converts other image types (WebP, GIF first frame, HEIC where the browser decodes it) to PNG or JPEG before uploading.
  - The longest edge is downscaled to 4096 px in the browser, and the server rejects anything over 10 MB.
- **Storage:** the blob store (`ArtifactBucket`) under `slides/<workspace>/<user>/<sha256>`, recorded in a new table:

  ```
  deck_assets(id, canvas_id FK → chat_canvases ON DELETE CASCADE, workspace_id, user_id, blob_key, mime, width, height, size_bytes, created_at)
  ```

- **Quota:** bytes count toward the person's existing 1 GB Library quota.
- **Serving:** `GET /slides/{canvasId}/assets/{assetId}`, for the owner only (404 for anyone else), with `Cache-Control: private, max-age=31536000, immutable`.
- **Lifecycle:**
  - An asset lives as long as its deck. Assets that no version references any more are kept, so restoring a version still works.
  - Deleting the deck cascades the rows, and a blob is deleted once no `deck_assets` row references its key.
  - Duplicating a deck copies the rows and shares the blobs.
- **Persona:** gets `slides_add_image` to import a chat attachment or generated image into the deck's assets. It can't fetch arbitrary URLs into a deck.

### 3.6 Persistence, autosave and conflicts

- **Reuse the canvas tables.** `chat_canvases.kind` gains `deck`. That needs a migration that widens the CHECK constraint and adds `deck_assets`, and a person must apply it (agents are denied `db:migrate`).
- Versions, `Restore`, `reviewed_seq`, ownership via `GetMyCanvas`, and the per-deck chat in an app space (`chat_spaces.app = 'slides'`) all come along unchanged.
- **Autosave:**
  - The editor saves the whole normalised deck with `Save(base, body)` 800 ms after the last edit and when the page is hidden or navigated away from.
  - Typing inside one text box is a single pending change until the box loses focus or 800 ms pass.
  - Save states and labels match Docs: Saved · Unsaved changes · Saving… · Not saved.
- **Undo/redo** is a client-side stack of ID-keyed ops for the session. Undoing after a save writes a new version. It never rewinds history.
- **Conflicts (409):**
  - The client does a three-way merge per element and per slide: the base is the version the client last saved, theirs is the new head, mine is the local deck.
  - Changes to different slides or elements merge automatically and save on top of the head.
  - Overlapping changes (the same element, the same slide's order or layout, or the theme) show a conflict banner like Docs: **Use the latest version**, **Keep mine** (overwrite only the overlapping items; theirs stays in history), or **Compare**, which opens the review view between the two.
  - Nothing is overwritten silently.
- While the persona is mid-turn on the deck ("Ada is editing…"), the editor is read-only, as in Docs.
- **Implementation risk:** each autosave is a version, and a deck body is typically 20–80 KB. If version volume matters in practice, squash consecutive versions by the same user within 2 minutes on the server. That's a follow-up and isn't required in v1.

### 3.7 Persona editing and review contract

- **Tools.** `canvas_read` on a deck returns the deck JSON. `canvas_patch` on a deck accepts deck ops, applied in order and atomically against `base` (stale base → 409, which the persona redoes after reading again):
  - `set_theme {theme}`
  - `insert_slide {after?: slideId, slide}`, where the server assigns missing IDs and builds placeholders from `layout` when `elements` is omitted
  - `set_slide {id, layout?, background?, notes?}`
  - `move_slide {id, after?: slideId}`
  - `remove_slide {id}`
  - `insert_element {slide, after?: elementId, element}`
  - `set_element {id, …fields}`, a merge patch where `paragraphs` replaces as a whole
  - `remove_element {id}`
  - `add_asset` happens only through `slides_add_image`.
- **Instructions.** The Slides space instructions tell the persona to:
  - read, then patch;
  - keep text within its box;
  - use theme tokens instead of hex;
  - never paste deck JSON into a reply.

  The persona can't see overflow in v1 (the layout runs in the browser). The review view flags overflow on persona changes instead.
- **Review.** This is `web/lib/slides/review.ts`, the deck counterpart of `doc-review.ts`, with the same `reviewBase` rule and the same `POST …/reviewed` endpoint. It's kind-agnostic. A neutral alias is added: `POST /canvases/{canvasId}/reviewed`.
  - One **decision per changed slide**: added, removed, moved, or modified (with an element-level change list: added, removed, moved/resized, text, style). A theme change is one more decision.
  - The view shows before and after thumbnails side by side, with changed elements outlined.
  - Accept or reject each decision, or all of them.
  - Resolving starts from head. Rejected slides go back to their base state and base position. A rejected added slide is removed, and a rejected removed slide is reinserted after its base predecessor.
  - The result is saved as a user version with the note "Kept n of Ada's m changes, rejected k", and `reviewed_seq` is advanced.
  - The person's own unsaved edits are flushed before the review is built. Review never runs on top of unsaved local state.

### 3.8 Shared rendering and export

- **One module renders everything.** `web/lib/slides/` holds the schema types, normalize, `layoutSlide` (the deck becomes a display list of rects, ellipses, lines, clipped images and positioned text pieces), and the renderers:
  - `<SlideSvg>` (React): editor canvas, thumbnails, review, presentation;
  - `toPdf(deck)`: jsPDF, lazy-loaded;
  - `toPptx(deck)`: pptxgenjs, lazy-loaded. It maps elements to native objects and reuses the layout constants, not the display list.
- All export runs in the browser, like DOCX today. The Go server doesn't need a renderer, a headless browser or fonts.
- **Fonts:** the Liberation TTFs ship in `web/public/fonts/slides/` (same origin), are loaded with `FontFace` before layout, and are embedded (subset) in the PDF.
- **Presentation:** `/apps/slides/{id}/present`.
  - It uses the Fullscreen API, and degrades to a full-window view when fullscreen is refused.
  - The SVG renderer is letterboxed to fit.
  - Keys:
    - → ↓ Space PgDn: next
    - ← ↑ PgUp: previous
    - Home / End: first / last
    - number + Enter: jump to a slide
    - Esc: exit
  - There's a visible **Exit** button and a slide counter. Hidden slides are skipped.
  - No transitions. Slides change instantly, which also covers `prefers-reduced-motion`.
  - Each slide is exposed as `role="img"` with an `aria-label` built from its text and image alt.

## 4. PDF and PPTX export: options compared

### 4.1 PDF

| Approach | Deps / licence | Runtime | Fonts | Delivery | Verdict |
|---|---|---|---|---|---|
| Browser print of a print page (what Docs does) | none | browser | browser | **The print dialog, not a file.** Page size and margins depend on the browser; Safari and Firefox handle `@page` size differently. | Rejected: no direct download, and page size isn't reliable |
| Server-side headless Chromium (`page.pdf`) | Chromium (BSD/various) | A ~300 MB browser in the API image, plus sandboxing, memory and concurrency limits | system or bundled | Direct download | Rejected: heavy runtime and ops burden, for output we can produce client-side |
| Server-side Go PDF (gofpdf: archived; others: AGPL or commercial) | — | Go | Embedding is possible | Direct download | Rejected: a second layout engine in Go would drift from the editor; licences are awkward |
| **Client-side jsPDF from the shared display list** | jsPDF 4.2.1 (MIT); no other runtime dep | Browser, lazy-loaded (~110 KB gzip) | Liberation TTFs embedded as subsets | **Blob → `<a download="Title.pdf">`: a direct download, no dialog** | **Chosen** |
| svg2pdf.js on top of jsPDF (MIT) | +1 dependency | browser | as above | as above | Not needed. The display list drives jsPDF directly, which the PoC shows is simpler and exact. |

**Delivery flow:** Export → PDF.
1. The editor flushes pending saves (as Docs' `current()` does).
2. It waits for fonts and images.
3. It builds the PDF in the browser and downloads `<Deck title>.pdf`.

The menu item says "Downloads a PDF", not "Opens the print dialog". The button shows progress while the PDF is built. A failure keeps the menu's error notice ("PDF export failed: …"), and the deck is untouched.

**Pagination:** one 960 × 540 pt page per visible slide, in deck order, with no splitting or scaling. Overflowing text is clipped at the page edge. Speaker notes aren't in the PDF in v1.

### 4.2 PPTX

| Approach | Licence | Editability | Verdict |
|---|---|---|---|
| **pptxgenjs, client-side** | MIT, 4.0.1, no runtime deps | Native `p:sp` text boxes with runs, `prstGeom` shapes, `p:pic` with `a:srcRect` crop, notes slides | **Chosen.** Same pattern as the `docx` export. |
| Hand-written OOXML | — | full control | Rejected for v1: pptxgenjs already writes valid OOXML for everything in the matrix |
| python-pptx or Go OOXML on the server | MIT / various | full | Rejected: adds a server-side renderer and duplicates layout knowledge |
| Rasterise each slide to an image | — | **none** | **Forbidden as the default** (ticket requirement). Not used even as a fallback for supported features. |

**Mapping:**

| Deck feature | PPTX |
|---|---|
| text element | text box `p:sp`, `wrap="square"`, insets of 8 pt, no autofit, `anchor` from valign |
| run | `a:r` with `b`, `i`, `u="sng"`, `sz` (pt × 100), `solidFill`, `latin typeface` (the slot's Office font) |
| paragraph | `algn`, exact `a:lnSpc/spcPts`, `spcBef`, `buChar •` or `buAutoNum arabicPeriod`, `lvl` with `marL`/`indent` |
| shape | `prstGeom` named after the shape (`rect`, `roundRect` with adj from radius, `ellipse`, and the presets in shapes/README.md at default adjust values), fill and line; text inside the same `p:sp` |
| line | `prstGeom line` with `tailEnd type="triangle"` |
| image | `p:pic`, `a:srcRect` from crop + cover (fractions), `descr` = alt |
| rotation | `a:xfrm rot` |
| background | `p:bg` solid fill |
| notes | notes slide |
| element ID | shape `name` |

**Limitations to accept and document:**
- Fonts aren't embedded in PPTX (pptxgenjs can't). Arial, Times New Roman and Courier New exist on Windows and macOS. LibreOffice maps them to Liberation, which is metric-identical. Where none of them exist, PowerPoint substitutes a font, and line breaks may differ.
- PowerPoint's own line breaking can differ from ours by a word at the margin when kerning or justification differs. We use left, centre and right alignment only, with no justify, to keep the difference small.
- Theme tokens are flattened to literal colours. The PPTX theme is PowerPoint's default, so changing the theme inside PowerPoint won't recolour the deck.

### 4.3 Features neither export can carry faithfully (so they're out of the v1 matrix)

Justified text, letter spacing, gradients, shadows, custom shape paths, tables, charts, image filters, and text on a path.

**Glyph coverage:** Liberation covers Latin, Greek and Cyrillic. Emoji and CJK are missing from Liberation, so they would fall back in the browser but render as missing glyphs in the PDF. In v1 the editor warns when text contains code points the slot font lacks. A Noto fallback face is a candidate follow-up.

## 5. Proof of concept (evidence)

`docs/slides/poc/` (run `npm ci && node run.mjs`, then `python -I verify.py "$PWD/out" "$PWD/evidence"` in a venv with `pymupdf` and `python-pptx`). It renders the shared acceptance deck in three ways from one layout module:

- `out/slide-N.svg` (editor and presentation renderer)
- `evidence/acceptance-deck.pdf` (5 pages, 133 KB)
- `evidence/acceptance-deck.pptx` (89 KB)

`verify.py` checks the outputs and writes `evidence/verify-report.json`. Results: **0 failures**.

**PDF:**
- 5 pages, each exactly 960 × 540 pt.
- Every drawn font is embedded: Liberation Sans, Liberation Serif, Liberation Mono.
- Titles and wrapped bullets are real, extractable text.
- Only the image slide contains a raster.

**PPTX:**
- 12 192 000 × 6 858 000 EMU, 5 slides in deck order.
- Every element's ID survives as its shape name, in paint order.
- Every text element, and every shape with text, is an editable text frame (python-pptx reads the runs and their fonts: Arial, Times New Roman, Courier New).
- The image is a `p:pic` with crop l=0.3325, r=0.0325, which matches the shared layout's cover+crop to 4 decimal places.
- No whole-slide picture, no `normAutofit`.
- Exact line spacing: 24, 28.8 and 48 pt.
- The level-1 bullet is at `lvl="1" marL="762000"` (= 120 units, as in the layout).
- A notes slide exists for the slide with notes.

**Visual:** `evidence/pdf-page-{1..5}.png` show:
- wrapped and mixed-format runs, disc and number bullets with hanging indents;
- serif and mono slots;
- the cover crop that trims the source's left red border;
- an 8° rotated shape with text, an arrow line;
- the overflow box spilling visibly (flagged by the layout as `s_shapes/e_s5 overflows by 104 units`).

**Findings from the PoC that shape implementation:**
1. jsPDF lists the 14 standard fonts and every image in a shared resource dictionary on every page. Checks must look at what a page draws, not at what it lists.
2. pptxgenjs measures `sizing.crop` against the picture's own `w × h`. Pass the source rectangle as fractions of the frame size.
3. pptxgenjs writes paragraph properties from each run's options. Every run in a paragraph must carry the same paragraph options.
4. The metric-compatible font pairs mean we don't need font embedding in PPTX for line breaks to match on machines that have Arial.

**Not verified here, and how it will be.** The sandbox has no PowerPoint or LibreOffice.
- **LibreOffice (automated):** the epic's QA ticket adds a CI step to the e2e stack:
  1. `soffice --headless --convert-to pdf acceptance-deck.pptx` must exit 0 and produce 5 pages.
  2. Each page is rendered and diffed against our PDF's page (allow small text-position tolerance).
  3. `soffice --convert-to pptx` round-trips without error.
- **PowerPoint (manual, recorded):** Liam (or QA) opens the exported acceptance deck in PowerPoint for Mac or Windows, confirms there's no "PowerPoint found a problem with content… Repair" prompt, edits a title and a shape's text, and attaches screenshots to the QA ticket. Optionally, the Open XML SDK validator (`dotnet` `OpenXmlValidator`, MIT) runs in CI as a schema check.

## 6. Reuse versus extension

**Reused unchanged:**
- canvas versions, `Restore` and seq conflicts;
- `GetMyCanvas` ownership;
- app spaces (`chat_spaces.app`);
- `Conversation` for the persona chat;
- `TopBar`, `Banner`, `Skeleton`, `Button`, the save-state labels;
- the `ExportMenu` structure;
- the `reviewed_seq` endpoint;
- `reviewBase()` (moved to a shared `web/lib/review-base.ts`).

**Extended:**
- `canvas.normalize` and `canvas.Apply` dispatch on kind `deck` to `slides.Normalize` and `slides.Apply(ops)`;
- MCP `canvas_read`, `canvas_patch` and `canvas_create` descriptions and schemas gain deck ops;
- `listMyCanvases` gains an optional `kind` filter;
- `MyCanvasKind` gains `deck`.

**New:**
- `server/internal/slides` (space and instructions, create/duplicate, normalize, ops, assets);
- `server/internal/httpapi/slides.go`;
- `web/lib/slides/*`;
- `web/app/(app)/apps/slides/**`;
- `deck_assets`.

**Migration and API implications:**
- **Migration** `000NN_slides.sql`:
  - widen `chat_canvases.kind` to `('text','svg','deck')`;
  - create `deck_assets`.
  - A person applies it.
- **New endpoints:**
  - `POST /slides` {title?, theme?} → canvas + chat
  - `POST /slides/{canvasId}/duplicate`
  - `POST/GET /slides/{canvasId}/assets[/{assetId}]`
  - `POST /canvases/{canvasId}/reviewed`, an alias that keeps `/docs/{id}/reviewed`
  - Rename and delete use the existing canvas title update and chat/canvas delete, or are added in the backend ticket if missing.
- **Enum growth.** `MyCanvasKind` and `ChatCanvas.kind` gain `deck`. Every `Record<kind, …>` map on the web and every place that switches on `kind` must handle it. The chat canvas panel shows a deck as a read-only thumbnail strip with **Open in Slides**. Docs already filters `kind === "text"`. Library "save canvas as source" refuses `deck` in v1.
- **MCP tool lists** change. Update the exact lists in `httpapi/mcp_profiles_test.go` and `chat_tools_test.go`.
- **Docs regression:** Docs code paths aren't modified, except moving `reviewBase` and adding the reviewed alias. `web/e2e/docs-app.spec.ts` must stay green.

## 7. Initial feature matrix (v1)

✓ = supported and verified against the acceptance deck · ⚠ = supported with the noted limitation · — = not in v1

| Feature | Editor | Present | PDF | PPTX (editable) |
|---|---|---|---|---|
| Add, duplicate, delete, reorder slides; hidden slides | ✓ | ✓ (skips hidden) | ✓ (skips hidden) | ✓ (hidden → `show="0"`) |
| Layouts: title, title-body, two-column, image-right, section, blank | ✓ | ✓ | ✓ | ✓ (placeholders become plain text boxes) |
| Themes (4 built-in; font slots and colour tokens) | ✓ | ✓ | ✓ | ⚠ colours flattened |
| Solid slide background | ✓ | ✓ | ✓ | ✓ |
| Text box: wrap, valign, insets | ✓ | ✓ | ✓ | ⚠ line breaks can differ by a word where the Office font is missing |
| Runs: bold, italic, underline, colour, size; sans/serif/mono | ✓ | ✓ | ✓ (embedded) | ✓ (Arial / Times New Roman / Courier New) |
| Paragraphs: left/centre/right, disc/number bullets, 5 levels, space before | ✓ | ✓ | ✓ | ✓ |
| Overflow | ✓ flagged | spills | spills, clipped at page | spills |
| Shapes: rect, rounded rect, ellipse and 18 presets (shapes/README.md); fill, stroke; text inside | ✓ | ✓ | ✓ | ✓ |
| Lines with arrowheads | ✓ | ✓ | ✓ | ✓ |
| Rotation | ✓ | ✓ | ✓ | ✓ |
| Images PNG/JPEG, crop + cover, alt text | ✓ | ✓ | ✓ | ✓ (`srcRect`, `descr`) |
| Speaker notes | ✓ (notes pane) | — (no presenter view in v1) | — | ✓ |
| Emoji / CJK | ⚠ warned | ⚠ | ⚠ missing glyphs | ⚠ font substitution |

**Deferred (confirm):**
- PPTX import;
- live multi-user collaboration (one editor plus the persona, with conflicts resolved as in §3.6);
- animations and transitions;
- embedded audio/video;
- presenter view with notes and timer;
- tables, charts, gradients, shadows, custom shapes, justified text;
- 4:3 and custom sizes;
- comments on slides;
- custom theme editor and uploaded fonts.

## 8. Representative acceptance deck

`docs/slides/fixtures/acceptance-deck.json` plus `fixtures/landscape.png`. Fifteen slides (the PoC evidence below covers the first five; the toolbar-controls, text-styles and layout slides came with later tickets), used unchanged by the editor tests, presentation tests, PDF tests and PPTX tests in every later ticket:

1. **Title:** surface background, bold heading-slot title aligned to the bottom, subtitle with mixed colour and italic runs, accent rule shape, speaker notes.
2. **Title + body:** disc bullets at levels 0 and 1, bold, italic, underline and literal-colour runs, a bullet long enough to wrap three lines, a numbered paragraph.
3. **Image right:** a 3:2 PNG cropped (`l=0.3`) and covered into a near-square frame, alt text, a serif italic paragraph with space before.
4. **Shapes:** rounded rect with centred bold white text, ellipse with stroke, a rectangle rotated 8° with text, an arrow line, and an over-full text box (the overflow case).
5. **Section:** dark background token, centred title, a mono run.
6. **Toolbar controls:** custom hex background, fill, outline and text colours; a rounded rect with radius 96 and a 20 px outline; a 2 px outline; a diagonal line with both arrowheads and a 1 px line with a start arrowhead; bullets at levels 0, 1, 2 and 4 in typed sizes (21, 15.5, 13 and 9 pt).
7. **Text styles:** a justified paragraph at 1.5 line spacing with strikethrough, hex and theme-colour highlight runs, and a double-spaced box that overflows only because of its line spacing. Line spacing exports as exact points (`a:spcPts`), not `a:spcPct`, since PowerPoint's 100% is the font's own line gap rather than the layout's 1.2.
8–15. **Layouts** ([[LAZYAGENTS-121]]): one slide per layout added in the second Slides epic, each placed exactly where its layout puts the placeholders: quote (italic heading-font body over a muted attribution), big number (a 280-unit accent stat over a centred caption), three columns (`body3`), comparison (`label`/`label2` headings over two bodies), full-bleed image (the landscape filling the slide under a white `#FFFFFF` title), image left, agenda (numbered body) and closing (centred title and contact line). The image layouts' images carry the `image` placeholder role. An unfilled image placeholder is a text element with that role and no text: every renderer draws it as nothing (the PPTX export skips it), the editor alone shows a "click to add image" box, and re-laying out to a layout without an image slot drops it.

## 9. Consequences

- **Good:**
  - One schema and one layout module drive the editor, presentation, PDF and PPTX. Differences are confined to PowerPoint's own line breaking.
  - The server stays renderer-free, and Docs' persistence, chat and review semantics carry over.
  - Stable IDs give per-element merge, review and undo.
- **Costs:**
  - We own a schema with validation (Go) and types (TS) that must stay in sync. A shared JSON fixture and round-trip tests in both languages guard this.
  - Three client additions: jsPDF (~110 KB gzip), pptxgenjs (~160 KB gzip) and the Liberation fonts (12 TTFs, ~1.6 MB, cached). All are lazy-loaded except the fonts the editor needs.
  - A migration that a person must apply.
- **Risks to watch:**
  - Editor text editing must measure exactly like `layoutSlide`. Use the same `FontFace` and `font-kerning: none`, and add a test that compares DOM line breaks with layout line breaks on the acceptance deck.
  - Version volume from autosave (§3.6).
  - Fonts in PPTX on machines without Arial.

## 10. Implementation status (LAZYAGENTS-103)

Liam approved this ADR and the mockups, then asked for the implementation in the same ticket, with PPTX optional. PPTX shipped too. What was built, and how it differs from the plan above:

- **Backend.**
  - Migration `00097_slides.sql` adds:
    - `deck` to `chat_canvases.kind`;
    - `slides` to `chat_spaces.app`, which the plan missed and the first end-to-end run caught;
    - the `deck_assets` table, keyed `(canvas_id, asset id)`, so a duplicate copies rows without rewriting the deck JSON.
  - `server/internal/deck` validates and normalises decks and applies persona ops.
  - `server/internal/slides` holds the space, create, duplicate, rename and image checks.
  - `httpapi/slides.go` serves `GET/POST /slides`, `PATCH/DELETE /slides/{id}`, `POST /slides/{id}/duplicate` and `POST/GET /slides/{id}/assets[/{assetId}]`.
  - Review reuses `POST /docs/{id}/reviewed`, which works for any canvas, so the neutral alias in §6 wasn't needed.
  - Comments on a deck are refused with a clear error, as deferred.
  - Agents can't `canvas_create` a deck; decks start in the Slides app.
- **Web.**
  - `web/lib/slides`: the schema, shared layout, fonts, SVG renderer, review and merge, and PDF/PPTX export.
  - Screens: deck list at `/apps/slides`, editor at `/apps/slides/{id}`, presentation at `/present/slides/{id}`. The presentation sits outside the app shell, like the print pages.
- **Fonts.** The bundle is the upstream Liberation 2.1.5 TTFs (OFL) under `web/public/fonts/slides`, 4.2 MB for 12 faces. Each face loads only when it's used.
- **SVG rendering.** Every slide is drawn with `text-rendering: geometricPrecision`. Hinted SVG text draws up to 2.5% narrower than the canvas measures at small scales, which opened gaps between words and made thumbnails overlap. Unhinted, the editor, thumbnails, presentation and PDF agree to within about 0.5%.
- **Image cleanup (follow-up N3, done in LAZYAGENTS-104).** Blobs are content-addressed per person (`slides/{workspace}/{user}/{sha256}`) and shared by a deck and its copies, so nothing deletes them inline any more.
  - Migration `00098_slides_blob_sweep.sql` adds a trigger on `deck_assets` that queues each released key in `deck_blob_sweep`. That covers every way a row goes: `DELETE /slides/{id}`, `DELETE /chats/{id}`, and a failed upload removing its own row.
  - The runner's sweep (`slides.SweepBlobs`, every 30 s, 100 keys a batch) handles keys queued over `slides.BlobSweepDelay` (1 hour) ago. It deletes a blob only when no `deck_assets` row uses the key; a copy, or a later upload of the same image, keeps it. A failed delete stays queued, moved to the back.
  - Uploads and the sweep take the same per-key advisory lock (`LockDeckBlob`) while inserting the row or checking and deleting the blob. So an upload either lands its row first, and the sweep keeps the blob, or writes the blob again after the sweep has deleted it.
  - Deleting the Slides space doesn't release images: its chats stay, outside any space, and the decks stay listed.
  - Quota is still counted from the rows, so queued blobs never count against it.
- **Not yet built.**
  - The conflict banner's **Compare** button: the banner offers Use the latest version and Keep mine.
  - The persona's `slides_add_image` tool: the persona edits text, shapes, layouts and themes, and the person adds images.
  - Renaming a deck doesn't rename its hidden chat, because renaming a chat waits for any running turn.
- **Verified by running the stack locally** (Postgres and SeaweedFS in Docker, API in no-sign-in mode, Next dev, Playwright):
  - create, rename, duplicate and delete;
  - stale saves return 409; invalid decks and non-images return 422;
  - editing text in place, inserting a shape and dragging it, image upload, undo and redo;
  - persona review with mixed accept and reject (version note and `reviewed_seq` checked);
  - a concurrent save on another slide merges, and on the same slide raises the banner without overwriting;
  - presenting with keyboard navigation and exit back to the editor;
  - a browser-made PDF with 5 pages at 960×540 pt and embedded fonts, and a browser-made PPTX with native, named shapes;
  - the narrow layout.

  Tests are deferred to the epic review. Opening the PPTX in PowerPoint and LibreOffice is still a manual or CI check, as §5 describes.
