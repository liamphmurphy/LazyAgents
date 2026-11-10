package runner

import (
	"context"
	"fmt"
	"net/url"
)

// Turns install tools with Nix, without root: the agent image ships Nix and
// points it at a daemon (NIX_REMOTE=daemon). One nix-daemon container per
// runner owns the store, which lives in a volume every turn mounts, so a
// package downloaded once is there for every later turn and run. Turns stay
// uid 1000 with no capabilities; the store is root-owned, so they can only
// add to it through the daemon. The daemon has Docker's default
// capabilities (it switches to its build users) and sits on the default
// bridge, not the agents network: it needs the binary cache, not the API.

const (
	nixMountPath = "/nix"
	nixDaemonBin = "/nix/var/nix/profiles/default/bin/nix-daemon"
)

func (d *Docker) nixDaemonName() string { return "lazyagents-nix-daemon-" + d.stack() }

// nixMount is the shared store as a turn mount, or nil when Nix is off.
func (d *Docker) nixMount() map[string]any {
	if d.NixVolume == "" {
		return nil
	}
	return map[string]any{"Type": "volume", "Source": d.NixVolume, "Target": nixMountPath}
}

// EnsureNixDaemon starts the runner's nix-daemon unless it's already running.
// The first container to mount an empty store volume copies the image's /nix
// into it, so the daemon starts from the Nix the image ships.
func (d *Docker) EnsureNixDaemon(ctx context.Context) error {
	if d.NixVolume == "" {
		return nil
	}
	name := d.nixDaemonName()
	var info struct{ State struct{ Running bool } }
	_, err := d.do(ctx, "GET", "/containers/"+url.PathEscape(name)+"/json", nil, "", &info)
	switch {
	case err == nil && info.State.Running:
		return nil
	case err == nil: // stopped (a host reboot): start it again below
	case isStatus(err, 404):
		// No run label: Cleanup removes leftover turns, and this isn't one.
		body := map[string]any{
			"Image": d.Image, "Cmd": []string{nixDaemonBin}, "User": "0",
			"Labels": map[string]string{stackLabelKey: d.stack()},
			"HostConfig": map[string]any{
				"Mounts":        []map[string]any{d.nixMount()},
				"Init":          true,
				"SecurityOpt":   []string{"no-new-privileges"},
				"RestartPolicy": map[string]any{"Name": "unless-stopped"},
			},
		}
		if _, err := d.do(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), jsonBody(body), "application/json", nil); err != nil && !isStatus(err, 409) {
			return fmt.Errorf("create nix-daemon: %w", err)
		}
	default:
		return fmt.Errorf("inspect nix-daemon: %w", err)
	}
	if _, err := d.do(ctx, "POST", "/containers/"+url.PathEscape(name)+"/start", nil, "", nil); err != nil && !isStatus(err, 304) {
		return fmt.Errorf("start nix-daemon: %w", err)
	}
	return nil
}
