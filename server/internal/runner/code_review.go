package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/pushreq"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A coding chat's Code dock shows each repository's branch against the
// default branch, committed and uncommitted, while the persona works and after
// the workspace is gone. The runner reads it with the same git it reports a
// turn's changes with (codechanges_run.go) and keeps the latest in
// chat_code_reviews.

// CodeReviewInterval is how often a running turn's workspace is checked for
// changes to show live.
var CodeReviewInterval = 5 * time.Second

// reviewCommitLimit bounds the commits a review lists.
const reviewCommitLimit = 100

// reviewSnapshot is one repository's review, as stored.
type reviewSnapshot struct {
	branch, base, head string
	commits            []pushreq.Commit
	files              []CodeChange
	dirty              bool
}

// gitLine is one trimmed line of git output, "" when the command fails.
func (r *Runner) gitLine(ctx context.Context, runID string, args ...string) string {
	out, _, code, err := r.gitCmd(ctx, runID, nil, 4<<10, args...)
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// reviewBase is where the branch left the remote's default branch: the
// merge base with origin/HEAD, else the commit the checkout was cloned at.
func (r *Runner) reviewBase(ctx context.Context, runID, head string) string {
	if base := r.gitLine(ctx, runID, "merge-base", head, "refs/remotes/origin/HEAD"); commitRe.MatchString(base) {
		return base
	}
	if base := r.gitLine(ctx, runID, "config", "--get", "vesper.base"); commitRe.MatchString(base) {
		return base
	}
	return head
}

// commitList is `git log base..head`, newest first.
func (r *Runner) commitList(ctx context.Context, runID, base, head string) []pushreq.Commit {
	if base == head {
		return []pushreq.Commit{}
	}
	out, _, code, err := r.gitCmd(ctx, runID, nil, 1<<20, "log", "--no-color", "--format=%H%x1f%s%x1e", "-n", "100", base+".."+head)
	commits := []pushreq.Commit{}
	if err != nil || code != 0 {
		return commits
	}
	for _, rec := range bytes.Split(out, []byte{0x1e}) {
		sha, subject, ok := strings.Cut(strings.TrimSpace(string(rec)), "\x1f")
		if ok && commitRe.MatchString(sha) && len(commits) < reviewCommitLimit {
			commits = append(commits, pushreq.Commit{SHA: sha, Subject: subject})
		}
	}
	return commits
}

// snapshot reads one checkout's review. ok is false when it has no commit yet.
func (r *Runner) snapshot(ctx context.Context, runID string, env map[string]string) (reviewSnapshot, bool, error) {
	head := r.gitLine(ctx, runID, "rev-parse", "--verify", "-q", "HEAD")
	if !commitRe.MatchString(head) {
		return reviewSnapshot{}, false, nil
	}
	s := reviewSnapshot{head: head, branch: r.gitLine(ctx, runID, "symbolic-ref", "--quiet", "--short", "HEAD")}
	s.base = r.reviewBase(ctx, runID, head)
	s.commits = r.commitList(ctx, runID, s.base, head)
	files, err := r.codeChanges(ctx, runID, s.base, env)
	if err != nil {
		return s, false, err
	}
	s.files = files
	filterEnv, err := r.repoFilterEnv(ctx, runID)
	if err != nil {
		return s, true, nil
	}
	status, _, code, err := r.gitCmd(ctx, runID, filterEnv, 1<<20, append([]string{"status", "--porcelain=v1", "-z", "--untracked-files=normal"}, codeChangePathspec...)...)
	s.dirty = err == nil && code == 0 && len(status) > 0
	return s, true, nil
}

// saveCodeReviews reads and stores the review of every checkout the chat has.
// Failures are logged and never fail the turn.
func (r *Runner) saveCodeReviews(ctx context.Context, c runCtx, env map[string]string, live bool) {
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		return
	}
	if !live {
		if err := r.q().SetChatCodeReviewsSettled(ctx, chat.ID); err != nil {
			r.Log.Warn("settle code reviews", "run", c.run.ID, "err", err)
		}
	}
	repos, err := r.q().ChatRepositories(ctx, chat.ID)
	if err != nil {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, codeChangesTimeout)
	defer cancel()
	for _, repo := range repos {
		s, ok, err := r.snapshot(inRepository(tctx, repo.Directory), c.run.ID.String(), env)
		if err != nil {
			r.Log.Warn("code review", "run", c.run.ID, "repo", repo.RepoUrl, "err", err)
		}
		if !ok {
			continue
		}
		commits, _ := json.Marshal(s.commits)
		files, _ := json.Marshal(s.files)
		var branch *string
		if s.branch != "" {
			branch = &s.branch
		}
		if err := r.q().UpsertChatCodeReview(ctx, store.UpsertChatCodeReviewParams{ChatID: chat.ID, RepoUrl: repo.RepoUrl, Branch: branch,
			BaseSha: &s.base, HeadSha: &s.head, Commits: commits, Files: files, Dirty: s.dirty, Live: live}); err != nil {
			r.Log.Warn("save code review", "run", c.run.ID, "err", err)
		}
	}
}

// reviewSignature changes whenever a checkout's commits, files or diff sizes
// do; the watcher refreshes the review only then. Three short git commands per
// checkout.
func (r *Runner) reviewSignature(ctx context.Context, c runCtx) string {
	chat, err := r.q().ChatSessionForRun(ctx, &c.run.ID)
	if err != nil {
		return ""
	}
	repos, err := r.q().ChatRepositories(ctx, chat.ID)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, repo := range repos {
		rc := inRepository(ctx, repo.Directory)
		id := c.run.ID.String()
		// Reading the working tree must never run the repository's programs.
		filterEnv, err := r.repoFilterEnv(rc, id)
		if err != nil {
			continue
		}
		status, _, _, _ := r.gitCmd(rc, id, filterEnv, 1<<20, append([]string{"status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all"}, codeChangePathspec...)...)
		numstat, _, _, _ := r.gitCmd(rc, id, filterEnv, 1<<20, append([]string{"diff", "--no-ext-diff", "--no-textconv", "--numstat", "-z", "HEAD"}, codeChangePathspec...)...)
		b.WriteString(repo.RepoUrl)
		b.Write(status)
		b.Write(numstat)
	}
	return b.String()
}

// watchCodeReview keeps a coding chat's review live while its turn runs, and
// returns a stop that waits for the watcher to finish.
func (r *Runner) watchCodeReview(ctx context.Context, c runCtx, env map[string]string) (stop func()) {
	if c.run.Purpose != "chat" || !allowedScope(c.spec, "repo:write") || CodeReviewInterval <= 0 {
		return func() {}
	}
	wctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(CodeReviewInterval)
		defer t.Stop()
		last := r.reviewSignature(wctx, c)
		for {
			select {
			case <-wctx.Done():
				return
			case <-t.C:
			}
			sig := r.reviewSignature(wctx, c)
			if sig == last || wctx.Err() != nil {
				continue
			}
			last = sig
			r.saveCodeReviews(wctx, c, env, true)
		}
	}()
	return func() { cancel(); wg.Wait() }
}

// withoutPushCredentials is a coding chat turn's environment without anything
// that can push: personas ask with request_push and the runner ships what the
// person approves. Setup, checkpoints and staging still get the credentials,
// in their own containers.
func withoutPushCredentials(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", envName("git:pat"), SSHKeyEnv} {
		delete(out, k)
	}
	out["GIT_TERMINAL_PROMPT"] = "0"
	return out
}

// gitToken is the workspace's GitHub token for the run's project, "" when none.
func (r *Runner) gitToken(ctx context.Context, c runCtx) (string, error) {
	if r.Secrets == nil {
		return "", nil
	}
	v, err := r.Secrets.Reveal(ctx, c.run.WorkspaceID, &c.project.ID, "git:pat")
	if errors.Is(err, apperr.ErrNotFound) || errors.Is(err, secrets.ErrNoKey) {
		return "", nil
	}
	return v, err
}

// chatSetupCredentialsDone ends the git part of a coding chat's setup: what
// follows (the repository's dependency installs, which run its own scripts)
// and the turn itself get no credential that can push. The SSH key file is
// removed too, as the turn shares the home volume it was written to.
func chatSetupCredentialsDone(c runCtx) string {
	if c.run.Purpose != "chat" || !allowedScope(c.spec, "repo:write") {
		return ""
	}
	return `rm -f "${` + SSHDirEnv + `:-$PWD/.lazyagents/ssh}/id"` + "\n" +
		"unset GH_TOKEN GITHUB_TOKEN " + envName("git:pat") + " " + SSHKeyEnv + "\n" +
		"export GIT_TERMINAL_PROMPT=0\n"
}
