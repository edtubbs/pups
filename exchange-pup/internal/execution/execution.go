package execution

import (
	"context"
	"fmt"

	"github.com/edtubbs/pups/exchange-pup/internal/config"
	"github.com/edtubbs/pups/exchange-pup/internal/node"
	"github.com/edtubbs/pups/exchange-pup/internal/storage"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type Result struct {
	Executor string `json:"executor"`
	Status   string `json:"status"`
	Details  any    `json:"details"`
}

type PaperExecutor struct {
	cfg   config.Config
	store *storage.Store
}

func NewPaper(cfg config.Config, store *storage.Store) *PaperExecutor {
	return &PaperExecutor{cfg: cfg, store: store}
}

func (p *PaperExecutor) Execute(ctx context.Context, rec types.Recommendation, symbol string, price float64, quantity float64) (Result, error) {
	fee := price * quantity * (p.cfg.PaperTrading.AssumedFeeBps / 10000)
	slip := price * quantity * (p.cfg.PaperTrading.AssumedSlippageBps / 10000)
	req := map[string]any{"signal_id": rec.SignalID, "symbol": symbol, "price": price, "quantity": quantity}
	resp := map[string]any{"fee": fee, "slippage": slip}
	if err := p.store.InsertExecution(ctx, rec.SignalID, "paper", "simulated", req, resp); err != nil {
		return Result{}, err
	}
	return Result{Executor: "paper", Status: "simulated", Details: resp}, nil
}

type ChainExecutor struct {
	rpc   *node.Client
	store *storage.Store
}

func NewChain(rpc *node.Client, store *storage.Store) *ChainExecutor {
	return &ChainExecutor{rpc: rpc, store: store}
}

func (c *ChainExecutor) Execute(ctx context.Context, rec types.Recommendation, destination string, amount float64, dryRun bool) (Result, error) {
	req := map[string]any{"destination": destination, "amount": amount, "dry_run": dryRun}
	if dryRun {
		_ = c.store.InsertExecution(ctx, rec.SignalID, "chain", "dry_run", req, map[string]any{"message": "dry run"})
		return Result{Executor: "chain", Status: "dry_run", Details: map[string]any{"message": "dry run"}}, nil
	}
	txid, err := c.rpc.ExecuteSendToAddress(ctx, destination, amount, "exchange-pup")
	if err != nil {
		_ = c.store.InsertExecution(ctx, rec.SignalID, "chain", "failed", req, map[string]any{"error": err.Error()})
		return Result{}, err
	}
	resp := map[string]any{"txid": txid}
	if err := c.store.InsertExecution(ctx, rec.SignalID, "chain", "submitted", req, resp); err != nil {
		return Result{}, err
	}
	return Result{Executor: "chain", Status: "submitted", Details: resp}, nil
}

type ExchangeExecutor struct{}

func NewExchange() *ExchangeExecutor { return &ExchangeExecutor{} }

func (e *ExchangeExecutor) Execute(_ context.Context, _ types.Recommendation) (Result, error) {
	return Result{}, fmt.Errorf("live exchange execution is scaffolded but disabled by default")
}
