import { COLOR_KEYS, DEFAULT_APPEARANCE, PALETTES, resolvePalette, type Appearance, type Palette } from "./palettes";

/** Before first paint. Resolver is self-contained and shared with live editing. */
export const themeScript = `(${function (palettes: readonly Palette[], defaults: Appearance, keys: string[], resolve: typeof resolvePalette) {
  const root = document.documentElement;
  let pref: "system" | "dark" | "light" = "system";
  let settings = defaults;
  try {
    const savedMode = localStorage.getItem("la-theme");
    if (savedMode === "light" || savedMode === "dark") pref = savedMode;
    const raw = localStorage.getItem("la-appearance");
    const saved = raw && raw.length <= 40000 ? JSON.parse(raw) : null;
    if (saved?.version === 1) {
      const known = (id: string) => palettes.some((p) => p.id === id);
      const clean = (input: Record<string, string>) => Object.fromEntries(keys.flatMap((key) => typeof input?.[key] === "string" && /^#[\da-f]{6}$/i.test(input[key]) ? [[key, input[key].toLowerCase()]] : []));
      settings = { version: 1, palette: saved.palette === "custom" || known(saved.palette) ? saved.palette : "dusk", custom: { base: known(saved.custom?.base) ? saved.custom.base : "dusk", dark: clean(saved.custom?.dark), light: clean(saved.custom?.light) } };
    }
  } catch { /* A corrupt preference or blocked storage must not prevent rendering. */ }
  if (pref !== "system") root.setAttribute("data-theme", pref);
  const mode = pref === "system" ? (matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark") : pref;
  const colors = resolve(settings, mode, palettes);
  for (const [key, value] of Object.entries(colors)) root.style.setProperty(`--${key}`, value);
  root.dataset.palette = settings.palette;
}.toString()})(${JSON.stringify(PALETTES)},${JSON.stringify(DEFAULT_APPEARANCE)},${JSON.stringify(COLOR_KEYS)},${resolvePalette.toString()});`;
