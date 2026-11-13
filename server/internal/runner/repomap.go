package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/testenv"
)

// A repo map is a short, generated overview of a ticket run's repository at
// its base commit (the default branch, or the epic branch for a stacked
// ticket), delivered as a context file so the agent can go straight to the
// right files instead of re-exploring the tree (RUNNER_REPO_MAP).
//
// It is deterministic: the tree, Makefile targets, package.json scripts,
// READMEs and Go package comments, all read from the commit itself, not the
// working tree. A model would write nicer prose but costs a call, can be
// wrong about the code, and isn't reproducible for a commit; the agent reads
// AGENTS.md for prose already. Maps are cached per repository and commit, so
// runs on the same base reuse one, and a moved base is a different commit and
// so a different map. Every map names its commit and when it was generated.

// RepoMapPath is where the map is delivered in the workspace.
const RepoMapPath = ".lazyagents/context/repo-map.md"

// baseCommitPrefix is the setup line that reports the run's base commit.
const baseCommitPrefix = "LAZYAGENTS_BASE_COMMIT="

// repoMapBudget caps the map's size, about 3.5k tokens.
const repoMapBudget = 14000

// baseCommitScript prints the commit the run's branch was cut from: the epic
// branch when the ticket stacks on it, else the base branch, else the remote's
// HEAD. It runs after the clone, so the refs are those of the clone.
func baseCommitScript(baseBranch, epicBranch string) string {
	var refs []string
	if epicBranch != "" {
		refs = append(refs, "origin/"+epicBranch)
	}
	if baseBranch != "" {
		refs = append(refs, "origin/"+baseBranch)
	}
	refs = append(refs, "origin/HEAD")
	quoted := make([]string, len(refs))
	for i, r := range refs {
		quoted[i] = shellQuote(r)
	}
	return "for ref in " + strings.Join(quoted, " ") + `; do if sha=$(git rev-parse -q --verify "refs/remotes/$ref^{commit}" 2>/dev/null); then echo "` + baseCommitPrefix + `$sha $ref"; break; fi; done` + "\n"
}

// takeBaseCommit removes the base commit line from setup output.
func takeBaseCommit(output string) (log, commit, ref string) {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		raw, ok := strings.CutPrefix(line, baseCommitPrefix)
		if !ok {
			lines = append(lines, line)
			continue
		}
		sha, name, _ := strings.Cut(strings.TrimSpace(raw), " ")
		if shaPattern.MatchString(sha) {
			commit, ref = sha, strings.TrimPrefix(name, "origin/")
		}
	}
	return strings.Join(lines, "\n"), commit, ref
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// Section markers of repoMapScript's output. Every content line is prefixed
// with "|", so a file can't forge a marker.
const (
	mapTree = "@@@ tree"
	mapDoc  = "@@@ godoc"
	mapFile = "@@@ file "
)

// repoMapScript prints what the map is built from, read at commit: the file
// list, the first Go package comment line of each file that has one, and the
// heads of the root build files and of READMEs and package.json files a few
// levels down. Vendored and generated trees are skipped.
func repoMapScript(commit string) string {
	c := shellQuote(commit)
	skip := `(^|/)(node_modules|vendor|third_party|dist|build)/`
	return `c=` + c + `
echo '` + mapTree + `'
git ls-tree -r --name-only "$c" | grep -vE '` + skip + `' | head -n 20000 | sed 's/^/|/'
echo '` + mapDoc + `'
git grep -I -m1 -e '^// Package ' "$c" -- '*.go' 2>/dev/null | sed "s|^$c:||" | grep -vE '` + skip + `' | head -n 3000 | sed 's/^/|/'
git ls-tree -r --name-only "$c" | grep -vE '` + skip + `' | grep -E '^(Makefile|makefile|GNUmakefile|go\.mod|package\.json|README(\.md)?|readme\.md)$|^([^/]+/){1,3}(README\.md|readme\.md|package\.json)$' | head -n 60 | while IFS= read -r f; do
  echo "` + mapFile + `$f"
  git show "$c:$f" 2>/dev/null | head -c 6000 | sed 's/^/|/'
  echo
done
true
`
}

// repoMapInput is what repoMapScript printed.
type repoMapInput struct {
	tree  []string
	docs  map[string]string // directory → first package comment line
	files map[string]string
}

func parseRepoMapOutput(out string) repoMapInput {
	in := repoMapInput{docs: map[string]string{}, files: map[string]string{}}
	section, file := "", ""
	var body strings.Builder
	flush := func() {
		if file != "" {
			in.files[file] = body.String()
		}
		file = ""
		body.Reset()
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == mapTree || line == mapDoc:
			flush()
			section = line
		case strings.HasPrefix(line, mapFile):
			flush()
			section, file = mapFile, strings.TrimPrefix(line, mapFile)
		case strings.HasPrefix(line, "|"):
			text := line[1:]
			switch section {
			case mapTree:
				in.tree = append(in.tree, text)
			case mapDoc:
				p, doc, ok := strings.Cut(text, ":")
				if !ok {
					continue
				}
				dir := path.Dir(p)
				if _, seen := in.docs[dir]; !seen {
					in.docs[dir] = strings.TrimSpace(strings.TrimPrefix(doc, "//"))
				}
			case mapFile:
				body.WriteString(text + "\n")
			}
		}
	}
	flush()
	return in
}

// buildRepoMap writes the map, labelled with its commit and generation time.
func buildRepoMap(repo, ref, commit string, at time.Time, in repoMapInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Repo map: %s\n\n", repo)
	on, moved := "", ""
	if ref != "" {
		on = " (`" + ref + "`)"
		moved = " If `git fetch` moves `origin/" + ref + "` past this commit, the base has changed since: `git diff --stat " + commit[:min(12, len(commit))] + " origin/" + ref + "` shows what."
	}
	fmt.Fprintf(&b, "Generated from commit `%s`%s on %s, the base this run's branch was cut from. "+
		"It is a starting point, not documentation.%s\n\n",
		commit, on, at.UTC().Format("2006-01-02 15:04 UTC"), moved)

	if len(in.tree) == 0 {
		b.WriteString("The repository has no files at this commit.\n")
		return b.String()
	}

	// Layout: every top-level entry, then the directories below it that say
	// what they are, then plain directories with many files.
	counts := map[string]int{}
	for _, f := range in.tree {
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			counts[d]++
		}
	}
	describe := func(dir string) string {
		if d, ok := in.docs[dir]; ok {
			return firstSentence(d)
		}
		if f, ok := in.files[dir+"/README.md"]; ok {
			return readmeSummary(f)
		}
		if f, ok := in.files[dir+"/package.json"]; ok {
			var pkg struct{ Description string }
			if json.Unmarshal([]byte(f), &pkg) == nil {
				return pkg.Description
			}
		}
		return ""
	}
	b.WriteString("## Layout\n\n")
	var top, rootFiles []string
	seenTop := map[string]bool{}
	for _, f := range in.tree {
		first, _, nested := strings.Cut(f, "/")
		if !nested {
			rootFiles = append(rootFiles, f)
		} else if !seenTop[first] {
			seenTop[first] = true
			top = append(top, first)
		}
	}
	var dirs []string
	for d := range counts {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, t := range top {
		line := fmt.Sprintf("- `%s/` (%d files)", t, counts[t])
		if d := describe(t); d != "" {
			line += ": " + d
		}
		b.WriteString(line + "\n")
		n := 0
		for _, d := range dirs {
			if !strings.HasPrefix(d, t+"/") || strings.Count(d, "/") > 3 {
				continue
			}
			desc := describe(d)
			if desc == "" && (strings.Count(d, "/") > 1 || counts[d] < 10) {
				continue
			}
			if n++; n > 80 {
				b.WriteString("  - …\n")
				break
			}
			line := fmt.Sprintf("  - `%s/` (%d)", d, counts[d])
			if desc != "" {
				line += ": " + desc
			}
			b.WriteString(line + "\n")
		}
	}
	if len(rootFiles) > 0 {
		if len(rootFiles) > 30 {
			rootFiles = append(rootFiles[:30], "…")
		}
		b.WriteString("- Root files: " + strings.Join(rootFiles, ", ") + "\n")
	}
	if s := readmeSummary(in.files["README.md"]); s != "" {
		b.WriteString("\nREADME: " + s + "\n")
	}

	// Entry points: main packages under cmd/ and package.json scripts.
	var mains []string
	for _, f := range in.tree {
		if path.Base(f) == "main.go" && (strings.Contains(f, "cmd/") || !strings.Contains(f, "/")) {
			mains = append(mains, "`"+path.Dir(f)+"`")
		}
	}
	mains = slices.Compact(mains)
	if len(mains) > 0 {
		if len(mains) > 20 {
			mains = append(mains[:20], "…")
		}
		b.WriteString("\n## Entry points\n\n- Go commands: " + strings.Join(mains, ", ") + "\n")
	}

	// Build and test.
	var build []string
	for _, mk := range []string{"Makefile", "makefile", "GNUmakefile"} {
		if f, ok := in.files[mk]; ok {
			if t := makeTargets(f); len(t) > 0 {
				build = append(build, "- `make` targets ("+mk+"): "+strings.Join(t, ", "))
			}
		}
	}
	var pkgs []string
	for p := range in.files {
		if path.Base(p) == "package.json" {
			pkgs = append(pkgs, p)
		}
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		var pkg struct{ Scripts map[string]string }
		if json.Unmarshal([]byte(in.files[p]), &pkg) != nil || len(pkg.Scripts) == 0 {
			continue
		}
		names := make([]string, 0, len(pkg.Scripts))
		for n := range pkg.Scripts {
			names = append(names, "`"+n+"`")
		}
		sort.Strings(names)
		if len(names) > 20 {
			names = append(names[:20], "…")
		}
		build = append(build, "- npm scripts ("+p+"): "+strings.Join(names, ", "))
	}
	if f, ok := in.files["go.mod"]; ok {
		if mod, _, _ := strings.Cut(f, "\n"); strings.HasPrefix(mod, "module ") {
			build = append(build, "- Go module `"+strings.TrimPrefix(mod, "module ")+"` at the root")
		}
	}
	for _, f := range in.tree {
		if path.Base(f) == "go.mod" && f != "go.mod" {
			build = append(build, "- Go module in `"+path.Dir(f)+"/`")
		}
	}
	if len(build) > 0 {
		b.WriteString("\n## Build and test\n\n" + strings.Join(build, "\n") + "\n")
	}

	// Conventions: where the repository writes them down.
	var conv []string
	for _, f := range []string{"AGENTS.md", "CLAUDE.md", "CONTRIBUTING.md", "docs", "doc"} {
		if slices.Contains(in.tree, f) || counts[f] > 0 {
			conv = append(conv, "`"+f+"`")
		}
	}
	if len(conv) > 0 {
		b.WriteString("\n## Conventions\n\nWritten down in " + strings.Join(conv, ", ") + " (AGENTS.md's repository part is already in your instructions).\n")
	}

	out := b.String()
	if len(out) > repoMapBudget {
		cut := strings.LastIndex(out[:repoMapBudget], "\n")
		out = out[:cut+1] + "\n…(map truncated)\n"
	}
	return out
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	if len(s) > 160 {
		s = strings.TrimSpace(s[:157]) + "…"
	}
	if s != "" && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "…") {
		s = strings.TrimRight(s, ",:;") + "…" // a sentence that carries on
	}
	return s
}

// readmeSummary is a README's first line of prose.
func readmeSummary(md string) string {
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence || t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "<") || strings.HasPrefix(t, "[!") || strings.HasPrefix(t, "![") || strings.HasPrefix(t, "|") || strings.HasPrefix(t, "---") {
			continue
		}
		return firstSentence(t)
	}
	return ""
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*)\s*:([^=]|$)`)

func makeTargets(makefile string) []string {
	var out []string
	for _, line := range strings.Split(makefile, "\n") {
		m := makeTarget.FindStringSubmatch(line)
		if m == nil || slices.Contains(out, "`"+m[1]+"`") {
			continue
		}
		out = append(out, "`"+m[1]+"`")
	}
	if len(out) > 30 {
		out = append(out[:30], "…")
	}
	return out
}

// repoMapCache keeps maps per repository and commit for this runner process.
// A map is cheap to rebuild from the workspace, so losing it on restart only
// costs one git read.
type repoMapCache struct {
	mu    sync.Mutex
	maps  map[string]string
	order []string
}

const repoMapCacheSize = 64

func (c *repoMapCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.maps[key]
	return m, ok
}

func (c *repoMapCache) put(key, m string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maps == nil {
		c.maps = map[string]string{}
	}
	if _, ok := c.maps[key]; !ok {
		c.order = append(c.order, key)
		if len(c.order) > repoMapCacheSize {
			delete(c.maps, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.maps[key] = m
}

// repoMapEvent is the lifecycle event that records a run's base commit.
const repoMapEvent = "repo_map"

// wantsRepoMap: top-level ticket runs with a repository, with the setting on.
func (r *Runner) wantsRepoMap(c runCtx) bool {
	return r.Cfg.RepoMap && c.run.Purpose == "ticket" && c.run.TicketID != nil && c.run.ParentRunID == nil &&
		c.project.RepoUrl != nil && *c.project.RepoUrl != ""
}

// repoMapFile is the run's map as a context file, or nil when it has none.
// The base commit is the one setup recorded; the map comes from the cache, or
// is built from the workspace's git, which has that commit, and cached.
func (r *Runner) repoMapFile(ctx context.Context, q *store.Queries, c runCtx, env map[string]string) (*ContextFile, error) {
	if !r.wantsRepoMap(c) {
		return nil, nil
	}
	commit, ref, err := runBaseCommit(ctx, q, c)
	if err != nil || commit == "" {
		return nil, err
	}
	repo := testenv.RepoKey(*c.project.RepoUrl)
	key := repo + "@" + commit
	if m, ok := r.repoMaps.get(key); ok {
		return &ContextFile{Path: RepoMapPath, Body: []byte(m)}, nil
	}
	out, err := r.runScript(ctx, c.run.ID, "lazyagents-repo-map", repoMapScript(commit), env)
	if err != nil {
		return nil, err
	}
	m := buildRepoMap(repo, ref, commit, time.Now(), parseRepoMapOutput(out))
	r.repoMaps.put(key, m)
	r.event(ctx, c.run.ID, "lifecycle", map[string]any{"state": "repo_map_generated", "commit": commit, "bytes": len(m)})
	return &ContextFile{Path: RepoMapPath, Body: []byte(m)}, nil
}

// runBaseCommit is the base commit setup recorded for the run. Setup records
// it before the first turn, so it is among the run's first events.
func runBaseCommit(ctx context.Context, q *store.Queries, c runCtx) (commit, ref string, err error) {
	events, err := q.ListRunEvents(ctx, store.ListRunEventsParams{RunID: c.run.ID})
	if err != nil {
		return "", "", err
	}
	for _, e := range events {
		if e.Type != "lifecycle" {
			continue
		}
		var p struct{ State, Commit, Ref string }
		if json.Unmarshal(e.Payload, &p) == nil && p.State == repoMapEvent && shaPattern.MatchString(p.Commit) {
			commit, ref = p.Commit, p.Ref // the latest: a restarted setup records it again
		}
	}
	return commit, ref, nil
}
