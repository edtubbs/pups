package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/attestation"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/blockchain"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/distribution"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/httpapi"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/identity"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/licensing"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/policy"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/telemetry"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/trust"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	log := logging.New(cfg.Logging.Level)
	if cfg.Policy.LiveAnchoringEnabled || cfg.Policy.TransfersEnabled {
		log.Warn("dangerous operations enabled by policy", "live_anchoring", cfg.Policy.LiveAnchoringEnabled, "transfers", cfg.Policy.TransfersEnabled)
	}

	db, err := storage.Open(ctx, cfg.Storage.SQLitePath)
	if err != nil {
		panic(fmt.Errorf("open storage: %w", err))
	}
	defer db.Close()

	metrics := telemetry.New()

	rpcClient := blockchain.NewClient(cfg.Blockchain)
	anchorSvc := blockchain.NewService(rpcClient, cfg.Policy.LiveAnchoringEnabled)
	identitySvc := identity.NewService(db, anchorSvc, log)
	attSvc := attestation.NewService(db, cfg.Attestation, log)
	trustSvc := trust.NewService(db, log)
	distSvc := distribution.NewService(cfg.Distribution, log)
	licSvc := licensing.NewService(db, cfg.Policy.TransfersEnabled, log)
	policyEngine := policy.NewEngine(cfg.Policy, log)

	h, err := httpapi.NewServer(httpapi.Dependencies{
		Config:       cfg,
		Logger:       log,
		Storage:      db,
		Metrics:      metrics,
		Identity:     identitySvc,
		Attestation:  attSvc,
		Trust:        trustSvc,
		Distribution: distSvc,
		Anchors:      anchorSvc,
		Licensing:    licSvc,
		Policy:       policyEngine,
	})
	if err != nil {
		panic(fmt.Errorf("create api: %w", err))
	}

	go func() {
		if err := h.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failure", "err", err)
			cancel()
		}
	}()
	go func() {
		if err := metrics.ListenAndServe(cfg.Runtime.MetricsBind); !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server failure", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()

	shutdownCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()

	_ = h.Shutdown(shutdownCtx)
	_ = metrics.Shutdown(shutdownCtx)
	log.Info("digital-asset-signing-pup stopped")
}
