---
name: ui-design-fidelity
description: "Use when building or changing any user-facing screen or component, to match the product's design system and cover layout, spacing, typography, color, theming and responsive behaviour."
---

# UI design fidelity

1. **Find the source of truth first:** a design doc (`DESIGN.md`), tokens file (`tokens.css`, Tailwind theme), mockups/screenshots, and existing components. Build from those. Don't invent colors, spacing, radii or fonts; if a needed token is missing, add it to the token file rather than hard-coding a value.
2. **Reuse before you create.** Search for an existing component (button, input, card, chip, table, menu) and extend its variants. A new component needs ≥2 intended uses or a distinct behaviour.
3. **Hierarchy over decoration.** One primary action per view; secondary actions visually quieter; destructive actions are not the default and are confirmed or undoable. Use type scale and weight for hierarchy, not extra colors. Align to the spacing scale (4/8px rhythm), keep consistent gutters and a clear reading width for text.
4. **Density and scanning:** lists/tables are for scanning: left-align text, right-align numbers (tabular figures), truncate with a tooltip rather than wrapping unpredictably, keep row heights consistent, show the most important column first.
5. **Color is meaning.** Use semantic tokens (success/warn/danger/info, state colors) consistently. Never rely on color alone: pair with an icon or text label. Check contrast (≥4.5:1 text, ≥3:1 UI boundaries/large text).
6. **Theming:** verify light *and* dark if the product supports both; use theme tokens (CSS variables), never raw hex in components.
7. **Responsive:** design mobile-first, test at ~375, 768, 1280px. No horizontal page scroll; tables get an `overflow-x-auto` wrapper or a stacked layout; touch targets ≥ 40×40px; don't hide essential actions behind hover.
8. **Motion:** subtle (100–200ms ease-out) and purposeful (state change, enter/exit). Respect `prefers-reduced-motion`.
9. **Copy:** sentence case, specific verbs ("Create ticket", not "Submit"), plain error messages that say what happened and what to do next, no blame, no jargon. Empty states explain the feature and give the primary action.
10. **Verify visually:** run the app, view the screen in the target states and breakpoints, compare to the mockup, and attach screenshots or a precise description to the PR.
