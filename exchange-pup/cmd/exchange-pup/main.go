package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/edtubbs/pups/exchange-pup/internal/config"
	"github.com/edtubbs/pups/exchange-pup/internal/execution"
	"github.com/edtubbs/pups/exchange-pup/internal/features"
	httpapi "github.com/edtubbs/pups/exchange-pup/internal/http"
	"github.com/edtubbs/pups/exchange-pup/internal/logging"
	"github.com/edtubbs/pups/exchange-pup/internal/market"
	"github.com/edtubbs/pups/exchange-pup/internal/model"
	"github.com/edtubbs/pups/exchange-pup/internal/node"
	"github.com/edtubbs/pups/exchange-pup/internal/policy"
	"github.com/edtubbs/pups/exchange-pup/internal/storage"
	"github.com/edtubbs/pups/exchange-pup/internal/strategy"
	"github.com/edtubbs/pups/exchange-pup/internal/telemetry"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type runtimeState struct {
	cfg      config.Config
	log      *slog.Logger
	store    *storage.Store
	policy   *policy.Engine
	node     *node.Client
	metrics  *telemetry.Metrics
	model    model.Engine
	modelMD  model.Metadata
	marketC  *market.Client
	adapters map[string]market.Adapter

	paperExec *execution.PaperExecutor
	chainExec *execution.ChainExecutor
	exchExec  *execution.ExchangeExecutor

	mu           sync.RWMutex
	latestRec    *types.Recommendation
	lastErr      string
	lastPoll     time.Time
	lastNodePoll time.Time
	freshMarket  bool
	freshModel   bool
	price        float64
	symbol       string
}

func main() {
	configPath := flag.String("config", envDefault("CONFIG_PATH", "/storage/exchange-pup.yaml"), "config path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	logger := logging.New("info")

	if cfg.Mode == types.ModeLiveChainExecute || cfg.Mode == types.ModeLiveExchangeExecute {
		logger.Warn("LIVE EXECUTION ENABLED - ensure explicit approvals and policy checks are configured")
	}

	store, err := storage.Open(cfg.SQLitePath, logger)
	if err != nil {
		log.Fatalf("storage error: %v", err)
	}
	defer store.Close()
	_ = store.SaveConfigSnapshot(context.Background(), cfg.Redacted())

	m := telemetry.New()

	r := &runtimeState{
		cfg:      cfg,
		log:      logger,
		store:    store,
		policy:   policy.New(cfg),
		node:     node.NewClient(cfg.NodeRPC),
		metrics:  m,
		marketC:  market.NewClient(),
		adapters: map[string]market.Adapter{},
		symbol:   firstOr(cfg.Market.Symbols, "DOGEUSDT"),
	}
	r.adapters["binance"] = market.NewBinance(r.marketC)
	r.adapters["kraken"] = market.NewKraken(r.marketC)
	r.paperExec = execution.NewPaper(cfg, store)
	r.chainExec = execution.NewChain(r.node, store)
	r.exchExec = execution.NewExchange()

	if err := r.initModel(); err != nil {
		logger.Error("model init failed", "err", err)
	}

	api := httpapi.New(cfg, store, r.policy, r)
	httpSrv := &http.Server{Addr: cfg.HTTPBind, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second}
	metricsSrv := &http.Server{Addr: cfg.MetricsBind, Handler: telemetry.Handler(), ReadHeaderTimeout: 5 * time.Second}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		r.log.Info("http server started", "addr", cfg.HTTPBind)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.log.Error("http server error", "err", err)
		}
	}()
	go func() {
		r.log.Info("metrics server started", "addr", cfg.MetricsBind)
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.log.Error("metrics server error", "err", err)
		}
	}()
	go r.loop(ctx)

	<-ctx.Done()
	shutdownCtx, c := context.WithTimeout(context.Background(), 8*time.Second)
	defer c()
	_ = httpSrv.Shutdown(shutdownCtx)
	_ = metricsSrv.Shutdown(shutdownCtx)
	r.log.Info("exchange-pup stopped")
}

func (r *runtimeState) initModel() error {
	backend := r.cfg.ModelBackend
	if backend == "" {
		backend = "fake"
	}
	if backend == "xgboost" {
		r.model = model.NewXGBoost()
	} else {
		r.model = model.NewFake()
	}

	md := model.Metadata{ModelVersion: "fake-v1", FeatureSchemaVersion: r.cfg.FeatureSchemaVersion, TargetLabels: []types.Action{
		types.ActionHold, types.ActionWaitNBlocks, types.ActionBuyNow, types.ActionSellNow, types.ActionMoveToExchange, types.ActionMoveToTreasury, types.ActionRebalance,
	}, Horizon: "1-3 blocks"}
	if r.cfg.ModelMetadataPath != "" {
		loaded, err := model.LoadMetadata(r.cfg.ModelMetadataPath)
		if err != nil {
			return err
		}
		md = loaded
	}
	if err := model.ValidateSchema(r.cfg.FeatureSchemaVersion, md.FeatureSchemaVersion); err != nil {
		return err
	}
	if err := r.model.LoadModel(r.cfg.ModelPath, md); err != nil {
		if r.model.Backend() == "xgboost" {
			return err
		}
	}
	r.modelMD = md
	r.freshModel = true
	r.metrics.ModelLoaded.Set(1)
	return nil
}

func (r *runtimeState) loop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.Market.RESTPollInterval)
	defer ticker.Stop()
	_ = r.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.tick(ctx)
		}
	}
}

func (r *runtimeState) tick(ctx context.Context) error {
	start := time.Now()
	rec, err := r.PredictNow(ctx)
	r.metrics.LatencyMS.WithLabelValues("pipeline").Observe(float64(time.Since(start).Milliseconds()))
	if err != nil {
		r.mu.Lock()
		r.lastErr = err.Error()
		r.mu.Unlock()
		r.metrics.APIErrorTotal.WithLabelValues("pipeline").Inc()
		return err
	}
	r.mu.Lock()
	r.latestRec = rec
	r.lastPoll = time.Now().UTC()
	r.mu.Unlock()
	return nil
}

func (r *runtimeState) PredictNow(ctx context.Context) (*types.Recommendation, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	nodeSnap, err := r.node.Snapshot(ctx)
	if err != nil {
		r.metrics.APIErrorTotal.WithLabelValues("node_rpc").Inc()
		return nil, fmt.Errorf("node snapshot: %w", err)
	}
	r.mu.Lock()
	r.lastNodePoll = time.Now().UTC()
	r.mu.Unlock()

	snap, err := r.pullMarket(ctx)
	if err != nil {
		return nil, err
	}
	r.price = snap.Tick.Price
	r.freshMarket = time.Since(snap.FetchedAt) <= r.cfg.Market.MaxDataStaleness

	for _, c := range snap.Candles {
		_ = r.store.InsertCandle(ctx, c)
	}
	_ = r.store.InsertTick(ctx, snap.Tick)
	_ = r.store.InsertOrderBook(ctx, snap.Book)

	fv := features.BuildFeatureVector(r.cfg.FeatureSchemaVersion, r.symbol, snap.Candles, snap.Tick, snap.Book, nodeSnap)
	if err := r.store.InsertFeatureRow(ctx, fv); err != nil {
		r.log.Warn("insert feature row failed", "err", err)
	}
	r.freshModel = r.freshModel && r.modelMD.FeatureSchemaVersion == fv.SchemaVersion

	inferStart := time.Now()
	ranked, conf, err := r.model.Predict(ctx, fv)
	r.metrics.LatencyMS.WithLabelValues("inference").Observe(float64(time.Since(inferStart).Milliseconds()))
	if err != nil {
		return nil, fmt.Errorf("predict: %w", err)
	}
	edge := expectedEdge(ranked)
	rec := strategy.BuildRecommendation(ranked, conf, edge, r.modelMD, fv)

	liq := sumBook(snap.Book)
	slip := fv.Values["slippage_proxy"] * 10000
	destination := fv.Context["target_bucket_id"]
	decision := r.policy.Evaluate(rec, destination, 100, r.freshMarket, r.freshModel, liq, slip)
	rec.PolicyDecision = decision
	if !decision.Allowed {
		r.metrics.PolicyRejects.Inc()
	}
	r.metrics.LatestSignalScore.Set(rec.Confidence)
	r.metrics.RecommendationsTotal.WithLabelValues(string(rec.Action)).Inc()

	_ = r.store.InsertPolicyDecision(ctx, rec.SignalID, decision)
	_ = r.store.InsertSignal(ctx, rec)
	return &rec, nil
}

func (r *runtimeState) pullMarket(ctx context.Context) (market.Snapshot, error) {
	var lastErr error
	for _, name := range r.cfg.Market.Exchanges {
		ad := r.adapters[name]
		if ad == nil {
			continue
		}
		start := time.Now()
		snap, err := ad.FetchSnapshot(ctx, r.symbol, firstOr(r.cfg.Market.CandleIntervals, "1m"))
		r.metrics.LatencyMS.WithLabelValues("exchange_" + name).Observe(float64(time.Since(start).Milliseconds()))
		if err == nil {
			return snap, nil
		}
		r.metrics.APIErrorTotal.WithLabelValues("exchange_" + name).Inc()
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no exchange adapters configured")
	}
	return market.Snapshot{}, lastErr
}

func (r *runtimeState) LatestStatus() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return map[string]any{
		"mode":                   r.cfg.Mode,
		"model_backend":          r.model.Backend(),
		"model_version":          r.modelMD.ModelVersion,
		"feature_schema_version": r.cfg.FeatureSchemaVersion,
		"fresh_market":           r.freshMarket,
		"fresh_model":            r.freshModel,
		"kill_switch":            r.policy.KillSwitch(),
		"last_error":             r.lastErr,
		"last_poll":              r.lastPoll,
		"last_node_poll":         r.lastNodePoll,
		"latest_signal":          r.latestRec,
	}
}

func (r *runtimeState) LatestRecommendation() *types.Recommendation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.latestRec == nil {
		return nil
	}
	cpy := *r.latestRec
	return &cpy
}

func (r *runtimeState) ExecuteSignal(ctx context.Context, signalID string) (map[string]any, error) {
	sigs, err := r.store.LatestSignals(ctx, 200)
	if err != nil {
		return nil, err
	}
	var rec *types.Recommendation
	for i := range sigs {
		if sigs[i].SignalID == signalID {
			t := sigs[i]
			rec = &t
			break
		}
	}
	if rec == nil {
		return nil, fmt.Errorf("signal not found")
	}
	if rec.PolicyDecision.RequiresApproval {
		return nil, fmt.Errorf("signal requires explicit approval")
	}

	switch r.cfg.Mode {
	case types.ModePaperTrade:
		res, err := r.paperExec.Execute(ctx, *rec, r.symbol, r.price, 100)
		if err != nil {
			return nil, err
		}
		r.metrics.ExecutionsTotal.WithLabelValues(res.Executor, res.Status).Inc()
		return map[string]any{"result": res}, nil
	case types.ModeLiveChainExecute:
		res, err := r.chainExec.Execute(ctx, *rec, "treasury", 100, r.cfg.DryRun)
		if err != nil {
			return nil, err
		}
		r.metrics.ExecutionsTotal.WithLabelValues(res.Executor, res.Status).Inc()
		return map[string]any{"result": res}, nil
	case types.ModeLiveExchangeExecute:
		res, err := r.exchExec.Execute(ctx, *rec)
		if err != nil {
			return nil, err
		}
		r.metrics.ExecutionsTotal.WithLabelValues(res.Executor, res.Status).Inc()
		return map[string]any{"result": res}, nil
	default:
		return nil, fmt.Errorf("execute disabled in recommend_only mode")
	}
}

func (r *runtimeState) ApproveSignal(ctx context.Context, signalID, approver, notes string, approved bool) error {
	return r.store.InsertApproval(ctx, signalID, approver, approved, notes)
}

func (r *runtimeState) PaperPerformance(_ context.Context) (map[string]any, error) {
	return map[string]any{
		"mode":                 r.cfg.Mode,
		"assumed_fee_bps":      r.cfg.PaperTrading.AssumedFeeBps,
		"assumed_slippage_bps": r.cfg.PaperTrading.AssumedSlippageBps,
		"starting_cash_doge":   r.cfg.PaperTrading.StartingCashDOGE,
	}, nil
}

func firstOr(v []string, d string) string {
	if len(v) == 0 {
		return d
	}
	return v[0]
}

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func expectedEdge(r []types.ScoredAction) float64 {
	if len(r) == 0 {
		return 0
	}
	if len(r) == 1 {
		return r[0].Score
	}
	return r[0].Score - r[1].Score
}

func sumBook(b types.OrderBook) float64 {
	s := 0.0
	for _, x := range b.Bids {
		s += x.Amount
	}
	for _, x := range b.Asks {
		s += x.Amount
	}
	return s
}
