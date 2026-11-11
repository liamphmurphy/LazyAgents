// Pure parsing for the chat composer's / commands. They run in the client and are
// never sent to the model; the actions live in chat/_components/conversation.tsx.

export type ChatCommand = { name: string; args?: string; hint: string };

export const CHAT_COMMANDS: ChatCommand[] = [
  { name: "model", args: "model", hint: "Switch the model for the next message" },
  { name: "effort", args: "effort", hint: "Set the reasoning effort for the next message" },
  { name: "persona", args: "persona", hint: "Start a new chat with another persona" },
  { name: "workflow", args: "workflow", hint: "Run one of your published workflows" },
  { name: "new", hint: "Start a new conversation" },
  { name: "repo", args: "repository", hint: "Select a repository in this conversation" },
  { name: "memory", hint: "Open your memory" },
  { name: "stop", hint: "Stop the current reply" },
];

/**
 * The command a finished message is, or null when it's an ordinary message. Only a
 * single line that starts with /word counts, so /usr/bin and multi-line text stay text.
 */
export function parseCommand(text: string): { name: string; arg: string } | null {
  const m = text.trimEnd().match(/^\/([a-z][a-z0-9-]*)(?:[ \t]+([^\n]*))?$/i);
  return m ? { name: m[1].toLowerCase(), arg: (m[2] ?? "").trim() } : null;
}

/** A message starting with // is sent as text, minus the first slash. */
export function literalBody(text: string): string {
  return text.startsWith("//") ? text.slice(1) : text;
}

/** What the suggestion menu should offer for the text so far: command names, then a command's arguments. */
export function commandMenu(text: string): { stage: "command"; query: string } | { stage: "arg"; name: string; query: string } | null {
  const name = text.match(/^\/([a-z0-9-]*)$/i);
  if (name) return { stage: "command", query: name[1] };
  const arg = text.match(/^\/([a-z][a-z0-9-]*)[ \t]+([^\n]*)$/i);
  if (!arg) return null;
  const command = CHAT_COMMANDS.find((c) => c.name === arg[1].toLowerCase());
  return command?.args ? { stage: "arg", name: command.name, query: arg[2] } : null;
}
