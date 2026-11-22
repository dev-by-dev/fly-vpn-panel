// flypanel is the VPN control panel binary: serve wires the full HTTP
// panel; version reports the build; admin manages the admin account.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flypanel/internal/api"
	"flypanel/internal/auth"
	"flypanel/internal/config"
	"flypanel/internal/inbounds"
	"flypanel/internal/monitoring"
	"flypanel/internal/stats"
	"flypanel/internal/store"
	"flypanel/internal/xray"
	"flypanel/internal/xrayclient"
)

const version = "v0.1.0-dev"

// xrayAPIAddr is the loopback address of xray's dokodemo-door gRPC API
// inbound (generated config, spec-fixed port).
const xrayAPIAddr = "127.0.0.1:10811"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("flypanel " + version)
	case "serve":
		serveCommand()
	case "admin":
		adminCommand(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

// serveCommand runs the panel: config → store → seed → JWT key → xray
// manager → poller/backup goroutines → HTTP server with graceful
// shutdown on SIGINT/SIGTERM.
func serveCommand() {
	cfg, err := config.Load()
	if err != nil {
		fatalf("serve: %v", err)
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		fatalf("serve: store: %v", err)
	}

	// Seed (idempotent) — the panel serves HTTP even without an admin.
	if _, err := st.AdminGet(); errors.Is(err, store.ErrNotFound) {
		log.Println("serve: no admin account yet — run flypanel admin init")
	} else if err != nil {
		fatalf("serve: admin: %v", err)
	}
	if err := api.Seed(st); err != nil {
		fatalf("serve: seed: %v", err)
	}
	keyB64, err := st.SettingGet("jwt_key")
	if err != nil {
		fatalf("serve: jwt_key: %v", err)
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		fatalf("serve: jwt_key: %v", err)
	}

	interval := api.PollInterval(st, cfg)
	mgr := xray.NewManager(cfg.XrayBin, cfg.DataDir+"/xray.json")
	xc, err := xrayclient.New(xrayAPIAddr)
	if err != nil {
		fatalf("serve: xray client: %v", err)
	}
	poller := stats.NewPoller(st, xc, xc, interval)

	// A broken or missing xray binary must not keep the panel down.
	if err := applyInitialXrayConfig(st, cfg, mgr); err != nil {
		log.Printf("serve: initial xray apply: %v (serving HTTP without xray)", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go poller.Run(ctx)
	go stats.RunBackup(ctx, st, cfg.DataDir)
	go monitoring.RunScraper(ctx, st, cfg.CaddyAdminURL, 15*time.Second, log.Printf)

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: api.NewRouter(api.Deps{
			Store:   st,
			JWT:     auth.NewJWT(key),
			RL:      auth.NewRateLimiter(5, time.Minute),
			Client:  xc,
			Manager: mgr,
			Cfg:     cfg,
			Poller:  poller,
		}),
		// Bound header reads so slow/idle connections cannot pin
		// goroutines forever (slowloris).
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("serve: listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		fatalf("serve: %v", err)
	case <-ctx.Done():
	}
	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("serve: shutdown: %v", err)
	}
	if err := mgr.Stop(); err != nil {
		log.Printf("serve: xray stop: %v", err)
	}
	if err := xc.Close(); err != nil {
		log.Printf("serve: xray client: %v", err)
	}
	if err := st.Close(); err != nil {
		log.Printf("serve: store close: %v", err)
	}
	log.Println("serve: stopped")
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: flypanel <version|serve|admin init>")
}

// applyInitialXrayConfig writes the xray config generated from the
// seeded store before xray first starts, so `xray run -c` has a valid
// file instead of crash-looping on a missing one. Apply ends with
// Mgr.Restart, which starts xray on the freshly written config.
func applyInitialXrayConfig(st *store.Store, cfg *config.Config, mgr *xray.Manager) error {
	routingJSON, err := st.SettingGet("routing")
	if err != nil {
		routingJSON = `{"domainStrategy":"AsIs","rules":[]}`
	}
	return inbounds.NewApplier(st, cfg, mgr).Apply(routingJSON)
}
