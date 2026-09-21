package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shreejaykurhade/MetaMax/backend/integrations/axl"
	"github.com/shreejaykurhade/MetaMax/backend/integrations/keeperhub"
	"github.com/shreejaykurhade/MetaMax/backend/integrations/zerog"
	"github.com/shreejaykurhade/MetaMax/backend/internal/api"
	"github.com/shreejaykurhade/MetaMax/backend/internal/auth"
	"github.com/shreejaykurhade/MetaMax/backend/internal/config"
	"github.com/shreejaykurhade/MetaMax/backend/internal/container"
	"github.com/shreejaykurhade/MetaMax/backend/internal/scanner"
	"github.com/shreejaykurhade/MetaMax/backend/internal/store"
)

func main() {
	cfg := config.Load()

	if cfg.PaymentsDisabled {
		log.Printf("[startup] WARNING: PAYMENTS_DISABLED=true — the x402 paywall on session creation is OFF. Do not use this setting on a public deployment.")
	}

	db, err := store.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	mgr, err := container.NewManager(cfg.DockerHost)
	if err != nil {
		log.Fatalf("container manager: %v", err)
	}

	sc := scanner.New(cfg.GroqAPIKey, cfg.ScanModel)

	authSvc := auth.New(cfg.JWTSecret)

	zeroGClient, err := zerog.New(cfg.ZeroG_RPC_URL, cfg.ZeroG_PrivateKey, cfg.ZeroG_FlowAddress)
	if err != nil {
		log.Fatalf("zerog: %v", err)
	}

	keeperClient, err := keeperhub.New(cfg.KeeperHub_Endpoint, cfg.KeeperHub_PrivateKey)
	if err != nil {
		log.Fatalf("keeperhub: %v", err)
	}

	// Gensyn AXL — agent-to-agent subtask delegation. Falls back to a no-op
	// client when AXL_ENDPOINT / AXL_PEER_ID are unset.
	axlClient, err := axl.New(cfg.AXL_Endpoint, cfg.AXL_PeerID)
	if err != nil {
		log.Fatalf("axl: %v", err)
	}

	srv := api.NewServer(cfg, db, mgr, sc, authSvc, keeperClient, zeroGClient, axlClient)

	// Start provider auto-bidder if PROVIDER_MODE=true.
	srv.StartProviderBidder(context.Background())

	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			authSvc.GCNonces()
		}
	}()

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Router(),
		// ReadHeaderTimeout, not ReadTimeout: ReadTimeout puts a deadline on the
		// whole exchange, so the connection is torn down while a slow handler is
		// still working — a repo scan waiting on the model would die at 30s with
		// "socket hang up", surfacing to the browser as a 500. Bounding only the
		// headers still turns away slowloris clients.
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("[server] listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("[server] shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("[server] shutdown error: %v", err)
	}
	log.Println("[server] stopped")
}
