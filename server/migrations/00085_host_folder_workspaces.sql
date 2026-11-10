-- +goose Up
-- Host-folder workspaces: a run's checkout can live in a folder on the
-- person's machine so they can open it in their own editor. The folder root
-- is deploy config on the runner (RUNNER_HOST_WORKSPACES_DIR), never set from
-- the API: a path that gets mounted into agent containers must not be
-- something an API caller can choose. The API only turns it on or off.
ALTER TABLE workspaces ADD COLUMN host_folder_workspaces boolean NOT NULL DEFAULT false;
-- What each runner has configured, so Settings can say whether the toggle
-- does anything ('' = not configured).
ALTER TABLE runners ADD COLUMN host_workspaces_dir text NOT NULL DEFAULT '';
-- The folder a run's checkout is in, when it is a host folder. Shown only
-- while the run still has its workspace (workspace_volume IS NOT NULL).
ALTER TABLE runs ADD COLUMN workspace_host_path text;

-- +goose Down
ALTER TABLE runs DROP COLUMN workspace_host_path;
ALTER TABLE runners DROP COLUMN host_workspaces_dir;
ALTER TABLE workspaces DROP COLUMN host_folder_workspaces;
