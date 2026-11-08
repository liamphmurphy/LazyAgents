// Command vesper is the single Vesper binary.
//
//	vesper serve        run migrations, ensure buckets, serve HTTP (default)
//	vesper migrate      run migrations and exit
//	vesper runner       execute dispatched runs (docker or local backend)
//	vesper healthcheck  exit 0 if the local API reports healthy (for container probes)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liamphmurphy/vesper/server/internal/agents"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/chatmodels"
	"github.com/liamphmurphy/vesper/server/internal/config"
	"github.com/liamphmurphy/vesper/server/internal/db"
	"github.com/liamphmurphy/vesper/server/internal/httpapi"
	"github.com/liamphmurphy/vesper/server/internal/memory"
	"github.com/liamphmurphy/vesper/server/internal/metrics"
	"github.com/liamphmurphy/vesper/server/internal/prsync"
	"github.com/liamphmurphy/vesper/server/internal/realtime"
	"github.com/liamphmurphy/vesper/server/internal/runner"
	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/secrets"
	"github.com/liamphmurphy/vesper/server/internal/seed"
	"github.com/liamphmurphy/vesper/server/internal/store"
	"github.com/liamphmurphy/vesper/server/internal/tracker"
	"github.com/liamphmurphy/vesper/server/internal/wiki"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd {
	case "serve":
		err = serve(ctx, log)
	case "migrate":
		err = migrateOnly(ctx)
	case "runner":
		err = runRunner(ctx, log)
	case "healthcheck":
		err = healthcheck(ctx)
	default:
		err = fmt.Errorf("unknown command %q (want serve|migrate|runner|healthcheck)", cmd)
	}
	if err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func connect(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func migrateOnly(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := connect(ctx, cfg)
	if err != nil {
		return err
	}
	pool.Close()
	return nil
}

func serve(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("migrations applied")

	blobs := blob.New(cfg.S3)
	bctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	err = blobs.EnsureBuckets(bctx)
	cancel()
	if err != nil {
		return err
	}

	arts := &artifacts.Service{Pool: pool, Blobs: blobs, Bucket: cfg.S3.ArtifactBucket}
	res, err := seed.Run(ctx, pool, seed.Options{Demo: cfg.Seed, UserName: cfg.LocalUserName, UserEmail: cfg.LocalUserEmail, Artifacts: arts})
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if res.Created {
		log.Info("seeded first workspace", "demo", cfg.Seed)
	}

	jobs, err := metrics.NewClient(pool, log)
	if err != nil {
		return err
	}
	if err = jobs.Start(ctx); err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = jobs.Stop(stopCtx)
	}()
	q := store.New(pool)
	box, err := secrets.NewBox(cfg.SecretsKey)
	if err != nil {
		return err
	}
	if box == nil {
		log.Warn("SECRETS_KEY is not set: storing secrets is disabled")
	}
	secretsSvc := &secrets.Service{Q: q, Box: box}
	trackerSvc := &tracker.Service{Pool: pool}
	events := realtime.New()
	go events.Run(ctx, pool)
	srv := &httpapi.Server{
		Events:  events,
		Version: cfg.Version,
		Checks:  map[string]httpapi.Pinger{"postgres": pool, "s3": blobs},
		Pool:    pool,
		Q:       q,
		Tracker: trackerSvc,

		Artifacts:      arts,
		Agents:         &agents.Service{Pool: pool},
		Secrets:        secretsSvc,
		Runs:           &runs.Service{Pool: pool, Secrets: secretsSvc, Artifacts: arts, Tracker: trackerSvc, RequireHarnessKey: cfg.Runner.Backend == "docker", OpenCodeIsolated: cfg.Runner.OpenCodeIsolated},
		Wiki:           &wiki.Service{Pool: pool, Artifacts: arts, Tracker: trackerSvc},
		PublicURL:      cfg.PublicURL,
		Blobs:          blobs,
		ArtifactBucket: cfg.S3.ArtifactBucket,
		Log:            log,
	}
	if cfg.PRSyncInterval > 0 {
		sync := &prsync.Syncer{Pool: pool, Secrets: secretsSvc, Tracker: trackerSvc, Log: log,
			MinAge: time.Duration(cfg.PRSyncInterval) * time.Second}
		go sync.Run(ctx, time.Duration(cfg.PRSyncInterval)*time.Second)
	}
	if cfg.MemoryConsolidateInterval > 0 {
		go memory.RunConsolidation(ctx, q, log, time.Duration(cfg.MemoryConsolidateInterval)*time.Second)
	}
	hs := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("listening", "addr", cfg.Addr, "version", cfg.Version)

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return hs.Shutdown(sctx)
	}
}

// healthcheck probes the running server. Distroless images have no curl.
func healthcheck(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	port := cfg.Addr
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/api/v1/health", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health: status %d", resp.StatusCode)
	}
	return nil
}

// runRunner executes dispatched runs (docs/dev/plan.md §6.4–6.6). It shares the
// database with the API and reaches it over /mcp like any agent.
func runRunner(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	box, err := secrets.NewBox(cfg.SecretsKey)
	if err != nil {
		return err
	}
	if box == nil {
		log.Warn("SECRETS_KEY is not set: runs get no secrets (harness keys, git:pat)")
	}
	rc := cfg.Runner
	var backend runner.Backend
	switch rc.Backend {
	case "docker":
		d := &runner.Docker{Socket: rc.DockerSocket, Image: rc.Image, Network: rc.Network, OpenCodeIsolated: rc.OpenCodeIsolated}
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := d.Ping(pctx)
		cancel()
		if err != nil {
			return fmt.Errorf("runner: docker: %w", err)
		}
		backend = d
	case "local":
		log.Warn("RUNNER_BACKEND=local runs harnesses on this machine with no isolation, using its own CLI logins")
		bins := map[string]string{}
		if rc.ClaudeBin != "" {
			bins["claude"] = rc.ClaudeBin
		}
		if rc.CodexBin != "" {
			bins["codex"] = rc.CodexBin
		}
		if rc.OpenCodeBin != "" {
			bins["opencode"] = rc.OpenCodeBin
		}
		backend = &runner.Local{Dir: rc.DataDir, Bins: bins, OpenCodeIsolated: rc.OpenCodeIsolated}
	}
	host, _ := os.Hostname()
	blobs := blob.New(cfg.S3)
	secretStore := &secrets.Service{Q: store.New(pool), Box: box}
	workflows := runs.NewWorkflowEngine(pool, &artifacts.Service{Pool: pool, Blobs: blobs, Bucket: cfg.S3.ArtifactBucket}, &tracker.Service{Pool: pool})
	workflows.Catalog = chatmodels.Catalog{RequireKey: rc.Backend == "docker", OpenCodeIsolated: rc.OpenCodeIsolated, Secrets: secretStore}
	r := &runner.Runner{
		Cfg: runner.Config{
			ModelPrices: rc.ModelPrices,
			ID:          fmt.Sprintf("%s-%d", host, os.Getpid()), MaxConcurrent: rc.MaxConcurrent, MCPURL: rc.MCPURL,
			Grace: 30 * time.Second, LogBucket: "run-logs", AttachmentBucket: cfg.S3.ArtifactBucket, Version: cfg.Version, RequireHarnessKey: rc.Backend == "docker",
			ChatIdleTTL: rc.ChatIdleTTL, MaxHold: rc.MaxHold, MaxHoldAttempts: rc.MaxHoldAttempts,
		},
		Pool: pool, Backend: backend, Secrets: secretStore,
		Tracker: &tracker.Service{Pool: pool}, Blobs: blobs, Attachments: blobs, Log: log,
		Workflows: workflows,
	}
	log.Info("runner started", "backend", rc.Backend, "slots", rc.MaxConcurrent, "mcp", rc.MCPURL)
	return r.Run(ctx)
}
