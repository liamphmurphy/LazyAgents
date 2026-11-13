package runner_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// repoMapOrigin points project PAY at a local origin with one commit and
// returns a function that commits a file there, and the commit it made.
func (e env) repoMapOrigin() (commit func(name string) string) {
	e.t.Helper()
	origin := filepath.Join(e.dir, "origin")
	if err := os.MkdirAll(origin, 0o700); err != nil {
		e.t.Fatal(err)
	}
	git := func(args ...string) string {
		e.t.Helper()
		out, err := exec.Command("git", append([]string{"-C", origin, "-c", "user.name=T", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
		if err != nil {
			e.t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	commit = func(name string) string {
		if err := os.WriteFile(filepath.Join(origin, name), []byte(name), 0o600); err != nil {
			e.t.Fatal(err)
		}
		git("add", ".")
		git("commit", "-qm", name)
		return git("rev-parse", "HEAD")
	}
	commit("README.md")
	if _, err := e.pool.Exec(e.ctx, "UPDATE projects SET repo_url=$1, default_branch='main' WHERE key='PAY'", origin); err != nil {
		e.t.Fatal(err)
	}
	return commit
}

// runEvents are a run's lifecycle events with the given state.
func (e env) lifecycle(runID uuid.UUID, state string) []map[string]any {
	evs, _ := e.q.ListRunEvents(e.ctx, store.ListRunEventsParams{RunID: runID})
	var out []map[string]any
	for _, ev := range evs {
		var p map[string]any
		if ev.Type == "lifecycle" && json.Unmarshal(ev.Payload, &p) == nil && p["state"] == state {
			out = append(out, p)
		}
	}
	return out
}

// contextFiles are the files delivered to a run's .lazyagents/context.
func (e env) contextFiles(run store.Run) []string {
	e.t.Helper()
	entries, err := os.ReadDir(filepath.Join(e.runner.Backend.WorkspaceDir(run.ID.String()), ".lazyagents", "context"))
	if err != nil {
		e.t.Fatal(err)
	}
	var names []string
	for _, d := range entries {
		names = append(names, d.Name())
	}
	return names
}

func (e env) idleRun(key string) store.Run {
	e.t.Helper()
	e.dispatch(key)
	e.until(key+" turn finished", func() bool {
		r := e.run(key)
		return r.State != "queued" && r.State != "starting" && len(e.turns(r.ID)) > 0 && e.turns(r.ID)[0].EndedAt != nil
	})
	return e.run(key)
}

// With the setting on, a ticket run gets a map of its base commit; another
// run on the same commit reuses the cached map, and a run after the base moved
// gets a new map that names the new commit.
func TestTicketRunsGetARepoMapOfTheirBaseCommit(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "idle")
	e.runner.Cfg.RepoMap = true
	commit := e.repoMapOrigin()
	first := commit("main.go")

	readMap := func(run store.Run) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(e.runner.Backend.WorkspaceDir(run.ID.String()), runner.RepoMapPath))
		if err != nil {
			t.Fatalf("no map: %v (events %s)", err, e.eventTypes(run.ID))
		}
		return string(b)
	}
	a := e.idleRun("PAY-214")
	m := readMap(a)
	if !strings.Contains(m, "Generated from commit `"+first+"` (`main`)") || !strings.Contains(m, "main.go") {
		t.Fatalf("map:\n%s", m)
	}
	if got := e.lifecycle(a.ID, "repo_map_generated"); len(got) != 1 || got[0]["commit"] != first {
		t.Fatalf("generated %v", got)
	}
	agentsMD, _ := os.ReadFile(filepath.Join(e.runner.Backend.WorkspaceDir(a.ID.String()), "AGENTS.md"))
	if !strings.Contains(string(agentsMD), runner.RepoMapPath) {
		t.Fatalf("AGENTS.md doesn't list the map:\n%s", agentsMD)
	}

	// Same base: the cached map, not a new one.
	b := e.idleRun("PAY-224")
	if readMap(b) != m || len(e.lifecycle(b.ID, "repo_map_generated")) != 0 {
		t.Fatalf("second run on %s regenerated: %v", first, e.lifecycle(b.ID, "repo_map_generated"))
	}

	// The base moves: the next run's map is for the new commit.
	moved := commit("moved.go")
	for _, r := range []store.Run{a, b} {
		_, _ = e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, r.ID, "cancel") // an idle run may have ended already
	}
	e.runner.Sweep(e.ctx)
	c := e.idleRun("PAY-214")
	if c.ID == a.ID {
		t.Fatal("no new run")
	}
	mc := readMap(c)
	if !strings.Contains(mc, "Generated from commit `"+moved+"`") || strings.Contains(mc, first) || !strings.Contains(mc, "moved.go") {
		t.Fatalf("map after the base moved:\n%s", mc)
	}
	if got := e.lifecycle(c.ID, "repo_map_generated"); len(got) != 1 || got[0]["commit"] != moved {
		t.Fatalf("generated %v", got)
	}
}

// With the setting off (the default), runs get the context files they always
// did: no map, no base commit read.
func TestRepoMapOffDeliversTheSameContext(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_HARNESS_MODE", "idle")
	e.repoMapOrigin()
	off := e.idleRun("PAY-214")
	if e.runner.Cfg.RepoMap {
		t.Fatal("the repo map is on by default")
	}
	if len(e.lifecycle(off.ID, "repo_map")) != 0 || len(e.lifecycle(off.ID, "repo_map_generated")) != 0 {
		t.Fatalf("events %s", e.eventTypes(off.ID))
	}
	without := e.contextFiles(off)
	if slices.Contains(without, filepath.Base(runner.RepoMapPath)) {
		t.Fatalf("context %v", without)
	}

	// The same ticket with the setting on gets exactly one more file.
	_, _ = e.runs.SetRunControl(e.ctx, e.ws, e.me.ID, off.ID, "cancel") // an idle run may have ended already
	e.runner.Sweep(e.ctx)
	e.runner.Cfg.RepoMap = true
	on := e.idleRun("PAY-214")
	with := e.contextFiles(on)
	if want := append(slices.Clone(without), filepath.Base(runner.RepoMapPath)); !slices.Equal(sorted(with), sorted(want)) {
		t.Fatalf("with the map %v, without %v", with, without)
	}
}

func sorted(s []string) []string { s = slices.Clone(s); slices.Sort(s); return s }
