package runner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testenv"
)

// A repository's test environment (testenv.Recipe) is read before its first
// turn, used at every turn, and grows as agents install tools. Services a
// person declared on the project win over the recipe's.

const testFilesPrefix = "LAZYAGENTS_TEST_FILES="

// testFilesScript prints the files testenv.Infer reads, as one JSON line the
// runner strips from the setup log. Capped well under the setup output limit.
func testFilesScript() string {
	globs, _ := json.Marshal(testenv.CandidateGlobs)
	py := `import glob, json
files, total = {}, 0
for pattern in ` + string(globs) + `:
    for p in sorted(glob.glob(pattern)):
        if p in files or len(files) >= 20:
            continue
        try:
            b = open(p, 'rb').read(65537)
        except OSError:
            continue
        if len(b) > 65536 or total + len(b) > 262144:
            continue
        files[p] = b.decode('utf-8', 'replace')
        total += len(b)
if files:
    print('` + testFilesPrefix + `' + json.dumps(files, separators=(',', ':')), flush=True)
`
	return "python3 -c " + shellQuote(py) + " || true\n"
}

// takeTestFiles removes the test files line from setup output.
func takeTestFiles(output string) (string, map[string][]byte) {
	var files map[string][]byte
	var log []string
	for _, line := range strings.Split(output, "\n") {
		raw, ok := strings.CutPrefix(line, testFilesPrefix)
		if !ok {
			log = append(log, line)
			continue
		}
		var text map[string]string
		if json.Unmarshal([]byte(raw), &text) == nil {
			files = map[string][]byte{}
			for k, v := range text {
				files[k] = []byte(v)
			}
		}
	}
	return strings.Join(log, "\n"), files
}

// testEnvRepo is the repository whose recipe applies to the run, as a
// testenv.RepoKey, or "" when the run has none.
func testEnvRepo(ctx context.Context, q *store.Queries, c runCtx) string {
	switch c.run.Purpose {
	case "ticket":
		if c.project.RepoUrl != nil {
			return testenv.RepoKey(*c.project.RepoUrl)
		}
	case "chat":
		if !allowedScope(c.spec, "repo:write") {
			return ""
		}
		if chat, err := q.ChatSessionForRun(ctx, &c.run.ID); err == nil && chat.RepoUrl != nil {
			return testenv.RepoKey(*chat.RepoUrl)
		}
	}
	return ""
}

// loadTestEnv is the repository's recipe; found is false when it has none.
func (r *Runner) loadTestEnv(ctx context.Context, c runCtx, repo string) (rec testenv.Recipe, found bool) {
	if repo == "" {
		return rec, false
	}
	row, err := r.q().GetTestEnvironment(ctx, store.GetTestEnvironmentParams{WorkspaceID: c.run.WorkspaceID, Repo: repo})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			r.Log.Warn("load test environment", "run", c.run.ID, "err", err)
		}
		return rec, false
	}
	if json.Unmarshal(row.Recipe, &rec) != nil || rec.Validate() != nil {
		// A recipe the rules no longer accept is ignored, not fatal.
		return testenv.Recipe{}, true
	}
	return rec, true
}

// testEnvServices are the services a turn starts: the project's, or else the
// repository's recipe.
func (r *Runner) testEnvServices(ctx context.Context, c runCtx) []agentdef.Service {
	if len(c.spec.Services) > 0 {
		return c.spec.Services
	}
	rec, _ := r.loadTestEnv(ctx, c, testEnvRepo(ctx, r.q(), c))
	return rec.Services
}

// inferTestEnv stores a first recipe from the files setup reported, when the
// repository has none yet.
func (r *Runner) inferTestEnv(ctx context.Context, c runCtx, repo string, files map[string][]byte) {
	if repo == "" || len(files) == 0 {
		return
	}
	rec := testenv.Infer(files)
	if rec.Empty() {
		return
	}
	b, _ := json.Marshal(rec)
	from := make([]string, 0, len(files))
	for p := range files {
		from = append(from, p)
	}
	if err := r.q().CreateInferredTestEnvironment(ctx, store.CreateInferredTestEnvironmentParams{
		WorkspaceID: c.run.WorkspaceID, Repo: repo, Recipe: b, InferredFrom: from,
	}); err != nil {
		r.Log.Warn("save inferred test environment", "run", c.run.ID, "err", err)
		return
	}
	names := make([]string, len(rec.Services))
	for i, s := range rec.Services {
		names[i] = s.Name + " (" + s.Image + ")"
	}
	r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "test_environment_inferred", "repo": repo, "services": names})
}

// NixManifestProcess is a finished turn that can say what its run's Nix
// profile holds.
type NixManifestProcess interface{ NixManifest() []byte }

// learnTestEnv adds the packages the turn's Nix profile holds to the
// repository's recipe, so the next run starts with them installed.
func (r *Runner) learnTestEnv(ctx context.Context, c runCtx, proc Process) {
	np, ok := proc.(NixManifestProcess)
	if !ok {
		return
	}
	pkgs := testenv.ParseNixManifest(np.NixManifest())
	repo := testEnvRepo(ctx, r.q(), c)
	if len(pkgs) == 0 || repo == "" {
		return
	}
	var added []string
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var rec testenv.Recipe
		row, err := q.LockTestEnvironment(ctx, store.LockTestEnvironmentParams{WorkspaceID: c.run.WorkspaceID, Repo: repo})
		switch {
		case err == nil:
			_ = json.Unmarshal(row.Recipe, &rec)
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		next, changed := rec.AddNixPackages(pkgs)
		if !changed {
			return nil
		}
		for _, p := range next.NixPackages {
			if !strings.Contains(" "+strings.Join(rec.NixPackages, " ")+" ", " "+p+" ") {
				added = append(added, p)
			}
		}
		b, _ := json.Marshal(next)
		return q.SaveLearnedTestEnvironment(ctx, store.SaveLearnedTestEnvironmentParams{
			WorkspaceID: c.run.WorkspaceID, Repo: repo, Recipe: b, LearnedFromRunID: &c.run.ID,
		})
	})
	if err != nil {
		r.Log.Warn("learn test environment", "run", c.run.ID, "err", err)
		return
	}
	if len(added) > 0 {
		r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "test_environment_learned", "repo": repo, "nix_packages": added})
	}
}
