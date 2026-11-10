# Color palettes

Settings → Appearance separates **Mode** (System, Light, Dark) from **Color palette**.
Dusk remains the default. Ten additional palette families offer paired light and dark adaptations.

| Palette | Basis / attribution |
| --- | --- |
| Catppuccin | [Catppuccin palette](https://catppuccin.com/palette/?flavor=latte), Mocha / Latte |
| Nord | [Nord](https://www.nordtheme.com/), Polar Night / Snow Storm |
| Dracula | [Dracula](https://draculatheme.com/contribute), Dracula / Alucard-inspired light |
| Gruvbox | [morhetz/gruvbox](https://github.com/morhetz/gruvbox), dark / light |
| Solarized | [Ethan Schoonover](https://ethanschoonover.com/solarized/), dark / light |
| Rosé Pine | [Rosé Pine](https://rosepinetheme.com/palette/), Main / Dawn |
| Tokyo Night | [folke/tokyonight.nvim](https://github.com/folke/tokyonight.nvim), Night / Day |
| Everforest | [sainnhe/Everforest](https://github.com/sainnhe/everforest/blob/master/README.md), medium dark / light |
| Ayu | [ayu-theme/ayu-colors](https://github.com/ayu-theme/ayu-colors), dark / light |
| Flexoki | [kepano/Flexoki](https://github.com/kepano/flexoki), dark / light |

These are Vesper adaptations rather than exact editor ports. The seed colors inform the background,
surfaces, text, accent, and eight series hues. Additional component colors are derived, and foregrounds
are adjusted for readable contrast. Presets meet 4.5:1 for text on the primary surfaces and button labels.
Custom palettes report contrast problems without preventing a person's intentional choices.

## Custom palettes

Select Custom to recover the last custom palette, or Customize this palette to start from the selected
preset. Light and dark overrides are independent. A radial hue/saturation wheel, a lightness slider,
and hex entry edit each color. Backgrounds, surfaces, foregrounds, buttons, states, brand colors,
agent/document colors, chart colors, and SVG roles are editable. Advanced overrides include borders,
hover and selection colors, and pill colors. Related hover states, tints, labels, and SVG roles follow
overridden base colors unless explicitly overridden themselves. Per-color, per-variant, and full resets
restore the base palette or Dusk. The preview always shows the variant being edited; Mode controls the app.

Preferences are local to this browser (the existing `la-theme` mode key is retained; versioned palette
settings use `la-appearance`). They survive reloads and synchronize across tabs. Export/import JSON
moves them between browsers/devices. There is no account synchronization. If storage is blocked, the
current visit still works and the editor explains how to export. Imports accept only known color keys
and six-digit opaque hex values; unknown fields and invalid colors are discarded.

## Rendering and exports

- The head script applies validated saved colors before first paint, using the same self-contained
  resolver as runtime edits. System changes update colors without a reload.
- UI SVG icons inherit currentColor. The orb and dependency edges use semantic tokens.
- Charts, themed SVG images/step-throughs, Mermaid diagrams, and syntax colors follow the selected palette.
- SVG downloads and Save to Library bake in the selected palette, so the image is self-contained.
- Explicit print views temporarily use Dusk Light for predictable paper output and restore the
  previous mode/palette on unmount. Saved settings are never overwritten by printing.
- Literal colors in arbitrary SVG illustrations, uploaded images, and independent sandboxed apps retain
  their original appearance. Such SVGs must use the `--viz-*` roles to participate in automatic recoloring.

## Implementation

`web/lib/palettes.ts` owns presets, validation, derivation, and contrast calculations.
`web/lib/palette-defaults.ts` captures the existing Dusk defaults from `tokens.css`, plus visualization
roles. Keep these aligned when modifying Dusk. Typography, spacing, and layout continue to use CSS.
`appearance-script.ts` serializes the preset table and self-contained resolver into the head script;
`components/shell/theme.tsx` owns live mode/palette changes, storage events, and OS updates.
`useVizPalette` resolves concrete CSS colors for SVG images, which cannot inherit variables from the page.
