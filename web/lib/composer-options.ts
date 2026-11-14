// The chat composer's option controls, described once: the desktop row lays them out inline
// and the compact composer's + sheet ("3 Composer + sheet") lists them, both from this.
// No React here so node --test can cover it.

import type { IconName } from "@/components/ui/icon";
import { repoShortName } from "./persona-ui.ts";

export type ComposerOptionId = "context" | "attach" | "code" | "repository" | "persona" | "send-mode" | "research" | "temporary" | "model" | "project";

/** How the control opens: a menu or popover of choices, a pressed/unpressed toggle, the file chooser, or a segmented choice. */
export type ComposerOptionOpens = "menu" | "popover" | "toggle" | "files" | "segmented";

/**
 * Where the desktop composer shows it: the option row (left), the actions before Send (right),
 * or the conversation header (the chat's repository and project), which the composer itself leaves out.
 */
export type ComposerOptionPlace = "options" | "actions" | "header";

export type ComposerOption = {
  id: ComposerOptionId;
  label: string;
  /** attach's paperclip isn't in the icon set; the attach button draws it. */
  icon: IconName | "paperclip";
  /** What it's set to now, e.g. the repository's short name; none when nothing is chosen. */
  value?: string;
  /** A toggle's state. */
  on?: boolean;
  opens: ComposerOptionOpens;
  place: ComposerOptionPlace;
};

export type ComposerOptionsState = {
  /** The persona can write code, so the chat works in a repository. */
  code: boolean;
  repo?: string;
  /** A repository chosen mid-turn, used from the next turn. */
  pendingRepo?: string;
  /** Before the first message, the persona can still be switched: its name. */
  persona?: string;
  /** While a reply runs, how Send sends (e.g. "Follow-up"). */
  sendMode?: string;
  research: boolean;
  /** Whether the chat is temporary, when that can still change; leave out otherwise. */
  temporary?: boolean;
  /** The model and effort summary ("Opus 5.5 · Auto"), once a model is selected. */
  model?: string;
  /** The chat's default project picker, when the chat has one: the project's name, if set. */
  project?: { name?: string };
};

/** The composer's options in desktop order: the row, then the actions, then the header's. */
export function composerOptions(s: ComposerOptionsState): ComposerOption[] {
  const list: ComposerOption[] = [
    { id: "context", label: "Add context", icon: "plus", opens: "menu", place: "options" },
    { id: "attach", label: "Attach files", icon: "paperclip", opens: "files", place: "options" },
    { id: "code", label: "Code or Markdown", icon: "code", opens: "popover", place: "options" },
  ];
  if (s.code) {
    const value = s.repo ? repoShortName(s.repo) : undefined;
    const next = s.pendingRepo ? `${repoShortName(s.pendingRepo)} (next turn)` : undefined;
    list.push({ id: "repository", label: "Repository", icon: "branch", value: next ? `${value ?? "none"} → ${next}` : value, opens: "popover", place: "header" });
  }
  if (s.persona !== undefined) list.push({ id: "persona", label: "Persona", icon: "users", value: s.persona, opens: "menu", place: "options" });
  if (s.sendMode !== undefined) list.push({ id: "send-mode", label: "Send as", icon: "msg", value: s.sendMode, opens: "segmented", place: "options" });
  list.push({ id: "research", label: "Research", icon: "search", on: s.research, opens: "toggle", place: "options" });
  if (s.temporary !== undefined) list.push({ id: "temporary", label: "Temporary", icon: "clock", on: s.temporary, opens: "toggle", place: "options" });
  if (s.model !== undefined) list.push({ id: "model", label: "Model and effort", icon: "sliders", value: s.model, opens: "menu", place: "actions" });
  if (s.project) list.push({ id: "project", label: "Default project", icon: "folder", value: s.project.name, opens: "menu", place: "header" });
  return list;
}
