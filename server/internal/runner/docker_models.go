package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// ModelLister lists the models a plan login offers. The Docker API image has
// no harness CLI, so the runner does it for the API (tickModelListings).
type ModelLister interface {
	// claim names the listing's container, so no two listings share one.
	// login is the saved harness secret; codexScope is where a refreshed
	// Codex login is stored (CodexAuthScope).
	ListModels(ctx context.Context, claim, harness, login, codexScope string) ([]chatmodels.Model, error)
}

const (
	listModelsTimeout = 60 * time.Second
	// maxConcurrentListings caps the throwaway containers listing at once.
	maxConcurrentListings = 4
	// listExitWait is how long the CLI gets to exit after its stdin closes.
	listExitWait = 5 * time.Second
)

// ListModels runs the harness CLI's model-listing protocol in a throwaway
// container with the sign-in sandbox (no volumes) and the saved login.
func (d *Docker) ListModels(ctx context.Context, claim, harness, login, codexScope string) ([]chatmodels.Model, error) {
	argv, err := chatmodels.DiscoveryArgv(harness)
	if err != nil {
		return nil, err
	}
	timeout := d.ModelsTimeout
	if timeout <= 0 {
		timeout = listModelsTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	env := []string{}
	if harness == agentdef.HarnessClaudeCode {
		env = append(env, ClaudeOAuthEnv+"="+login)
	}
	body := map[string]any{
		"Image": d.Image, "Cmd": argv, "Env": env, "WorkingDir": homePath,
		"Labels": d.labels("models-" + harness),
		"Tty":    false, "OpenStdin": true, "StdinOnce": true, "AttachStdin": true, "AttachStdout": true, "AttachStderr": true,
		"HostConfig": d.hostConfig([]map[string]any{}),
	}
	var created struct{ ID string }
	name := url.QueryEscape("vesper-models-" + claim)
	if _, err := d.do(ctx, "POST", "/containers/create?name="+name, jsonBody(body), "application/json", &created); err != nil {
		return nil, err
	}
	started := false
	var stream io.ReadWriteCloser
	defer func() {
		// The caller's context may be what ended; cleanup must still happen.
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		if started {
			// Closing stdin makes the CLI exit. Codex may rotate its token until
			// then, so the login is only read back once the container is done.
			stream.Close()
			wctx, wcancel := context.WithTimeout(context.Background(), listExitWait)
			_, _ = d.do(wctx, "POST", "/containers/"+created.ID+"/wait", nil, "", nil)
			wcancel()
			if harness == agentdef.HarnessCodex && d.SaveCodexAuth != nil {
				// Codex rotates its refresh token, so a refreshed login must be kept.
				if next, ok := d.refreshedCodexAuth(cctx, created.ID, login); ok {
					if err := d.SaveCodexAuth(cctx, codexScope, next); err != nil {
						slog.Warn("save refreshed codex login", "err", err)
					}
				}
			}
		}
		_, _ = d.do(cctx, "POST", "/containers/"+created.ID+"/kill?signal=SIGKILL", nil, "", nil)
		_, _ = d.do(cctx, "DELETE", "/containers/"+created.ID+"?force=1", nil, "", nil)
	}()
	if harness == agentdef.HarnessCodex {
		// A login never travels in the environment: it is a file, as in turns.
		tb, err := codexAuthTar(login)
		if err != nil {
			return nil, err
		}
		if _, err := d.do(ctx, "PUT", "/containers/"+created.ID+"/archive?path="+url.QueryEscape(homePath), tb, "application/x-tar", nil); err != nil {
			return nil, err
		}
	}
	st, err := d.attachLogin(created.ID, true)
	if err != nil {
		return nil, err
	}
	stream = st
	defer st.Close()
	if _, err := d.do(ctx, "POST", "/containers/"+created.ID+"/start", nil, "", nil); err != nil {
		return nil, err
	}
	started = true
	// Closing the stream is what unblocks a read when the time runs out.
	go func() { <-ctx.Done(); st.Close() }()
	r, w := io.Pipe()
	go func() { w.CloseWithError(demux(st, w, io.Discard)) }()
	defer r.Close()
	return chatmodels.ListOverStdio(harness, st, r)
}

// tickModelListings answers the model-listing requests the API queued for plan
// logins (chatmodels.QueuedListings). Like sign-ins they take no run slot.
func (r *Runner) tickModelListings(ctx context.Context) {
	ml, ok := r.Backend.(ModelLister)
	if !ok || r.Secrets == nil {
		return
	}
	q := r.q()
	for r.listings.Add(1) <= maxConcurrentListings {
		l, err := q.ClaimModelListing(ctx, &r.Cfg.ID)
		if err != nil {
			r.listings.Add(-1)
			if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
				r.Log.Error("claim model listing", "err", err)
			}
			return
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			defer r.listings.Add(-1)
			r.runModelListing(ctx, ml, l)
		}()
	}
	r.listings.Add(-1) // the attempt that found the cap reached
}

func (r *Runner) runModelListing(parent context.Context, ml ModelLister, l store.HarnessModelListing) {
	q := r.q()
	// The caller's context may be what ended; the outcome is still recorded.
	// Each write gets its own deadline: listing can outlast any shared one.
	record := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	}
	fail := func(msg string) {
		ctx, cancel := record()
		defer cancel()
		if _, err := q.FailModelListing(ctx, store.FailModelListingParams{ClaimID: l.ClaimID, Error: msg}); err != nil {
			r.Log.Warn("fail model listing", "workspace", l.WorkspaceID, "harness", l.Harness, "err", err)
		}
	}
	label := "Claude"
	if l.Harness == agentdef.HarnessCodex {
		label = "ChatGPT"
	}
	login, err := r.Secrets.Reveal(parent, l.WorkspaceID, nil, chatmodels.KeySecret[l.Harness])
	if parent.Err() != nil {
		return // shutting down: the claim times out and another runner retries
	}
	planLogin := chatmodels.ClaudeOAuthToken(login)
	if l.Harness == agentdef.HarnessCodex {
		_, planLogin = chatmodels.ParseCodexAuth(login)
	}
	if err != nil || !planLogin {
		fail("The saved login isn't a Claude or ChatGPT plan login.")
		return
	}
	models, err := ml.ListModels(parent, l.ClaimID.String(), l.Harness, login, CodexAuthScope(l.WorkspaceID, nil))
	if err != nil {
		// The error can carry CLI output; keep it out of the database.
		r.Log.Warn("list models", "workspace", l.WorkspaceID, "harness", l.Harness, "err", err)
		if parent.Err() == nil {
			fail("Couldn't list models with the saved " + label + " login. Sign in again in Settings.")
		}
		return
	}
	if len(models) == 0 {
		fail("No chat models available.")
		return
	}
	b, err := json.Marshal(models)
	if err != nil {
		fail("Couldn't list models with the saved " + label + " login. Sign in again in Settings.")
		return
	}
	ctx, cancel := record()
	defer cancel()
	if _, err := q.FinishModelListing(ctx, store.FinishModelListingParams{ClaimID: l.ClaimID, Models: b}); err != nil {
		r.Log.Warn("finish model listing", "workspace", l.WorkspaceID, "harness", l.Harness, "err", err)
	}
}
