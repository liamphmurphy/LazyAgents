// Image engine choices, as the Settings, persona and workflow editors word them.
// Pure so node --test covers it.

export type EngineChoice = "auto" | "codex" | "antigravity";
export type EngineOption = { engine: "codex" | "antigravity"; label: string; available: boolean; reason?: string };

/** The radios in Settings: Auto first, then each engine with whether it is connected. */
export function engineChoices(options: EngineOption[]): { value: EngineChoice; label: string; detail: string; disabled: boolean }[] {
  const connected = options.filter((o) => o.available);
  const auto = connected.length
    ? `Use the first connected: ${connected.map((o) => o.label).join(", then ")}.`
    : "No image engine is connected yet. Connect Codex or Antigravity first.";
  return [
    { value: "auto", label: "Auto", detail: auto, disabled: false },
    ...options.map((o) => ({ value: o.engine, label: o.label, detail: o.available ? "Connected" : o.reason ? `Not connected: ${o.reason}` : "Not connected", disabled: false })),
  ];
}

/** What an engine setting means for a persona or workflow step: "" inherits the level above. */
export function engineSummary(value: string | undefined, inherited: EngineChoice, options: EngineOption[]): string {
  const label = (v: string) => (v === "auto" ? "Auto" : (options.find((o) => o.engine === v)?.label ?? v));
  return !value ? `Workspace setting (${label(inherited)})` : label(value);
}
