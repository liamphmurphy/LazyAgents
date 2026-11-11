"use client";

import { useState, type ReactNode, type RefObject } from "react";
import type { Schemas } from "@/lib/api/client";
import { addChip, chipFromTarget, removeChip, type Chip } from "@/lib/chat-context";
import { CHAT_COMMANDS, commandMenu, literalBody, parseCommand } from "@/lib/chat-commands";
import { insertMention, mentionQuery, mentionSuggestions } from "@/lib/persona-ui";
import { cx } from "@/lib/cx";
import { LinkEditor } from "@/components/domain/link-editor";
import { acceptTarget, AddContext, ChipList } from "./context-chips";
import { AttachButton, AttachmentChips, type useAttachments } from "./attachments";
import { ComposerHints, composerFrameFor, composerInput, SendButton, type SendTone } from "./reply-tools";
import { PersonaAvatar, PersonaName, personaLook } from "./persona";

/** A / command's action; a returned string is why it couldn't run. */
export type CommandRun = () => string | void;

export type ComposerCommands = {
  repository?: (arg: string) => string | void;
  /** The commands that take nothing after them: new, memory, stop. */
  plain: Record<string, CommandRun>;
  /** What a command that takes something offers, e.g. the models for /model. */
  options: (name: string) => { name: string; run: CommandRun }[];
  hasWorkflows: boolean;
  /** The commands whose argument is text to send (/aside, /queue). */
  free?: Record<string, (text: string) => string | void>;
};

/**
 * The one message box, for a conversation and for the welcome before its first message:
 * [[ context and Add context, files, @ persona mentions, / commands, then the caller's own
 * options (left) and actions (right, before Send). Its parent owns the draft, chips and files
 * so it can send them, clear them and seed them from links.
 */
export function Composer({
  text,
  onText,
  inputRef,
  files,
  chips,
  onChips,
  onSubmit,
  commands,
  personas,
  inChat,
  placeholder,
  disabled,
  rows = 1,
  options,
  actions,
  send,
  onCycle,
  hints,
}: {
  text: string;
  onText: (text: string) => void;
  inputRef: RefObject<HTMLTextAreaElement | null>;
  files: ReturnType<typeof useAttachments>;
  chips: Chip[];
  onChips: (update: (chips: Chip[]) => Chip[]) => void;
  /** A message that isn't a command, with a leading // already reduced to /. */
  onSubmit: (body: string) => void;
  commands: ComposerCommands;
  /** Who @ can mention; inChat marks the ones already in the conversation. */
  personas: Schemas["Persona"][];
  inChat: (personaId: string) => boolean;
  placeholder: string;
  disabled: boolean;
  rows?: number;
  options?: ReactNode;
  actions?: ReactNode;
  send: { label: string; disabled: boolean; title?: string; tone?: SendTone };
  /** Tab in a draft with no menu open: the caller switches how Send sends (back for Shift+Tab). */
  onCycle?: (back: boolean) => void;
  /** Replaces the usual line of hints under the box. */
  hints?: ReactNode;
}) {
  // The @mention being typed, and which suggestion is highlighted.
  const [mention, setMention] = useState<{ start: number; query: string } | null>(null);
  const [mentionActive, setMentionActive] = useState(0);
  // The / command menu: which row is highlighted, whether Esc closed it, and why the last command didn't run.
  const [commandActive, setCommandActive] = useState(0);
  const [commandDismissed, setCommandDismissed] = useState(false);
  const [commandError, setCommandError] = useState<string | null>(null);

  const attach = (t: Parameters<typeof chipFromTarget>[0]) => {
    const chip = chipFromTarget(t);
    if (chip) onChips((cs) => addChip(cs, chip));
  };
  const menu = commandDismissed ? null : commandMenu(text);
  const suggestions = mention && !menu ? mentionSuggestions(personas, mention.query) : [];
  const syncMention = (value: string) => {
    const el = inputRef.current;
    const next = el ? mentionQuery(value, el.selectionStart) : null;
    if (next?.query !== mention?.query) setMentionActive(0);
    setMention(next);
  };
  const pickMention = (name: string) => {
    const el = inputRef.current;
    if (!el || !mention) return;
    const next = insertMention(text, mention.start, el.selectionStart, name);
    onText(next.text);
    setMention(null);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(next.caret, next.caret);
    });
  };

  const runCommand = (run: CommandRun) => {
    const problem = run();
    setCommandError(problem || null);
    if (!problem) onText("");
  };
  const commandItems: { name: string; label: string; hint?: string; choose: () => void }[] = !menu
    ? []
    : menu.stage === "arg"
      ? mentionSuggestions(commands.options(menu.name), menu.query, 8).map((o) => ({ name: o.name, label: o.name, choose: () => runCommand(o.run) }))
      : mentionSuggestions(CHAT_COMMANDS, menu.query, CHAT_COMMANDS.length).map((cmd) => ({
          name: cmd.name,
          label: `/${cmd.name}`,
          hint: cmd.hint,
          choose: () => {
            setCommandActive(0);
            if (cmd.args) onText(`/${cmd.name} `);
            else runCommand(commands.plain[cmd.name]);
          },
        }));
  // A finished command, typed in full or with the menu closed.
  const runTyped = ({ name, arg }: { name: string; arg: string }) => {
    const command = CHAT_COMMANDS.find((x) => x.name === name);
    if (!command) return setCommandError(`Unknown command /${name}. Start with // to send it as a message.`);
    if (name === "repo" && commands.repository) return runCommand(() => commands.repository!(arg));
    const free = command.free ? commands.free?.[name] : undefined;
    if (free) return arg ? runCommand(() => free(arg)) : setCommandError(`Say what to send: /${name} <${command.args}>`);
    if (!command.args) return arg ? setCommandError(`/${name} doesn’t take anything after it.`) : runCommand(commands.plain[name]);
    if (name === "workflow" && !commands.hasWorkflows) return setCommandError("You have no published workflows yet. Build one under Workflows.");
    if (!arg) return setCommandError(`Say which ${command.args}: /${name} <${command.args}>`);
    const match = mentionSuggestions(commands.options(name), arg)[0];
    if (!match) return setCommandError(`No ${command.args} matches “${arg}”.`);
    runCommand(match.run);
  };
  const submitComposer = () => {
    const command = parseCommand(text);
    if (command) runTyped(command);
    else onSubmit(literalBody(text));
  };

  return (
    <>
      {commandError && (
        <p role="alert" data-testid="command-error" className="text-[12.5px] text-pill-blocked-fg">
          {commandError}
        </p>
      )}
      <form
        data-dock-avoid=""
        onPaste={files.onPaste}
        onSubmit={(e) => {
          e.preventDefault();
          submitComposer();
        }}
        className="flex flex-col gap-2"
      >
        <AttachmentChips attachments={files} />
        <ChipList chips={chips} onRemove={(k) => onChips((cs) => removeChip(cs, k))} disabled={disabled} />
        <div className={composerFrameFor(send.tone)}>
          <div className="relative">
            {commandItems.length > 0 && (
              <ul
                role="listbox"
                aria-label="Commands"
                data-testid="command-suggestions"
                className="absolute bottom-full left-2 z-10 mb-1 flex min-w-[280px] flex-col rounded-lg border border-border bg-surface-raised p-1 shadow-lg"
              >
                {commandItems.map((item, i) => (
                  <li key={item.name} role="option" aria-selected={i === commandActive}>
                    <button
                      type="button"
                      onMouseDown={(e) => e.preventDefault()}
                      onMouseEnter={() => setCommandActive(i)}
                      onClick={item.choose}
                      className={cx("flex w-full items-center gap-3 rounded-md px-2 py-1.5 text-left text-[13px]", i === commandActive && "bg-selected")}
                    >
                      <span className="flex-none">{item.label}</span>
                      {item.hint && <span className="ml-auto truncate text-xs text-fg-tertiary">{item.hint}</span>}
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {suggestions.length > 0 && (
              <ul
                role="listbox"
                aria-label="Mention a persona"
                data-testid="mention-suggestions"
                className="absolute bottom-full left-2 z-10 mb-1 flex min-w-[200px] flex-col rounded-lg border border-border bg-surface-raised p-1 shadow-lg"
              >
                {suggestions.map((p, i) => (
                  <li key={p.id} role="option" aria-selected={i === mentionActive}>
                    <button
                      type="button"
                      onMouseDown={(e) => e.preventDefault()}
                      onMouseEnter={() => setMentionActive(i)}
                      onClick={() => pickMention(p.name)}
                      className={cx("flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px]", i === mentionActive && "bg-selected")}
                    >
                      <PersonaAvatar persona={personaLook(p)} size={18} />
                      <PersonaName persona={personaLook(p)} />
                      {inChat(p.id) && <span className="ml-auto text-xs text-fg-tertiary">in chat</span>}
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <LinkEditor
              inputRef={(el) => {
                inputRef.current = el;
              }}
              mono={false}
              aria-label="Message"
              className={composerInput}
              rows={rows}
              value={text}
              onChange={(v) => {
                onText(v);
                syncMention(v);
                setCommandActive(0);
                setCommandDismissed(false);
                setCommandError(null);
              }}
              onBlur={() =>
                setTimeout(() => {
                  if (document.activeElement === inputRef.current) return;
                  setMention(null);
                  setCommandDismissed(true);
                }, 120)
              }
              onPick={attach}
              accept={acceptTarget}
              disabled={disabled}
              placeholder={placeholder}
              onKeyDown={(e) => {
                if (commandItems.length > 0) {
                  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                    e.preventDefault();
                    setCommandActive((a) => (a + (e.key === "ArrowDown" ? 1 : commandItems.length - 1)) % commandItems.length);
                    return;
                  }
                  if ((e.key === "Enter" || e.key === "Tab") && !e.shiftKey && !e.nativeEvent.isComposing) {
                    e.preventDefault();
                    commandItems[Math.min(commandActive, commandItems.length - 1)].choose();
                    return;
                  }
                  if (e.key === "Escape") {
                    e.preventDefault();
                    setCommandDismissed(true);
                    return;
                  }
                }
                if (suggestions.length > 0) {
                  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                    e.preventDefault();
                    setMentionActive((a) => (a + (e.key === "ArrowDown" ? 1 : suggestions.length - 1)) % suggestions.length);
                    return;
                  }
                  if ((e.key === "Enter" || e.key === "Tab") && !e.nativeEvent.isComposing) {
                    e.preventDefault();
                    pickMention(suggestions[Math.min(mentionActive, suggestions.length - 1)].name);
                    return;
                  }
                  if (e.key === "Escape") {
                    e.preventDefault();
                    setMention(null);
                    return;
                  }
                }
                // Only with a draft, so Tab still leaves an empty box for the next control.
                if (e.key === "Tab" && onCycle && text.trim() && !e.altKey && !e.ctrlKey && !e.metaKey) {
                  e.preventDefault();
                  onCycle(e.shiftKey);
                  return;
                }
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  submitComposer();
                }
              }}
            />
          </div>
          {/* The options wrap within their own column so model, Stop and Send never drop to a line of their own. */}
          <div className="flex items-end gap-2">
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
              <AddContext onPick={attach} onClosed={() => inputRef.current?.focus()} disabled={disabled} compact />
              <AttachButton onFiles={files.add} disabled={disabled} compact />
              {options}
            </div>
            <div className="flex flex-none items-center gap-1">
              {actions}
              <SendButton label={send.label} disabled={send.disabled} title={send.title} tone={send.tone} />
            </div>
          </div>
        </div>
      </form>
      <ComposerHints>{hints}</ComposerHints>
    </>
  );
}
