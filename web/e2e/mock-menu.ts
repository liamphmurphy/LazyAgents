/**
 * The main menu is on every page: its chat history, personas, spaces and
 * scheduled tasks. Mocks that only serve one screen answer these with empty
 * lists (undefined: not a menu request), so they still catch everything else.
 */
export function menuResponse(method: string, path: string): unknown {
  if (method !== "GET") return undefined;
  if (path === "/chats") return { chats: [], has_more: false };
  if (path === "/personas") return { personas: [] };
  if (path === "/chat-spaces") return { spaces: [] };
  if (path === "/scheduled-tasks") return { tasks: [] };
  return undefined;
}
