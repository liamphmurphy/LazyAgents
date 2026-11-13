-- +goose Up
-- The chat tools a chat segment was last told it has. A segment keeps its CLI
-- session across messages, so when Vesper gains tools (or a persona gains
-- capabilities) the next message tells the model what changed instead of
-- starting a fresh session. NULL for runs that aren't chats, and for chat
-- segments from before this column, which are told nothing until they change.
ALTER TABLE runs ADD COLUMN chat_tool_names text[];

-- +goose Down
ALTER TABLE runs DROP COLUMN chat_tool_names;
