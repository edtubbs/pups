package policy

import (
	"strings"
	"sync"
	"time"

	"github.com/edtubbs/pups/exchange-pup/internal/config"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type Engine struct {
	cfg        config.Config
	mu         sync.Mutex
	killSwitch bool
	lastAction time.Time
	dailyStart time.Time
	dailyUsed  float64
}

func New(cfg config.Config) *Engine {
	return &Engine{cfg: cfg, killSwitch: cfg.KillSwitchInitialState, dailyStart: time.Now().UTC()}
}

func (e *Engine) SetKillSwitch(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.killSwitch = v
}

func (e *Engine) KillSwitch() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.killSwitch
}

func (e *Engine) Evaluate(rec types.Recommendation, destination string, amountDOGE float64, marketFresh bool, modelFresh bool, liquidity float64, slippageBps float64) types.PolicyDecision {
	e.mu.Lock()
	defer e.mu.Unlock()

	d := types.PolicyDecision{Allowed: true, Mode: e.cfg.Mode, ReasonCodes: []string{}, RequiresApproval: false, DryRun: e.cfg.DryRun}

	if e.killSwitch {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "KILL_SWITCH_ON")
	}
	if e.cfg.RecommendOnlyMasterSwitch {
		d.ReasonCodes = append(d.ReasonCodes, "RECOMMEND_ONLY_MASTER_SWITCH")
	}
	if !marketFresh {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "STALE_MARKET_DATA")
	}
	if !modelFresh {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "STALE_MODEL_OR_SCHEMA")
	}
	if rec.Confidence < e.cfg.Policy.MinConfidence {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "MIN_CONFIDENCE_FAILED")
	}
	if rec.ExpectedEdge < e.cfg.Policy.MinExpectedEdge {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "MIN_EDGE_FAILED")
	}
	if liquidity < e.cfg.Policy.MinLiquidity {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "MIN_LIQUIDITY_FAILED")
	}
	if slippageBps > e.cfg.Policy.MaxSlippageBps {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "MAX_SLIPPAGE_EXCEEDED")
	}
	if amountDOGE > e.cfg.Policy.MaxSingleActionAmountDOGE {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "MAX_SINGLE_AMOUNT_EXCEEDED")
	}

	now := time.Now().UTC()
	if now.Sub(e.lastAction) < e.cfg.Policy.Cooldown {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "COOLDOWN_ACTIVE")
	}
	if now.Sub(e.dailyStart) > 24*time.Hour {
		e.dailyStart = now
		e.dailyUsed = 0
	}
	if e.dailyUsed+amountDOGE > e.cfg.Policy.MaxDailyAmountDOGE {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "MAX_DAILY_AMOUNT_EXCEEDED")
	}

	if !isWhitelisted(destination, e.cfg.Policy.WhitelistedAddresses, e.cfg.Policy.WhitelistedBuckets) {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "DESTINATION_NOT_WHITELISTED")
	}

	if e.cfg.RequireHumanApproval && rec.Confidence >= e.cfg.RequireHumanApprovalOverConf {
		d.RequiresApproval = true
		d.ReasonCodes = append(d.ReasonCodes, "HUMAN_APPROVAL_REQUIRED")
	}

	if e.cfg.Mode == types.ModeRecommendOnly {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "MODE_RECOMMEND_ONLY")
	}
	if e.cfg.PaperTradeOnlyMasterSwitch && e.cfg.Mode != types.ModePaperTrade {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "PAPER_ONLY_MASTER_SWITCH")
	}
	if e.cfg.Mode == types.ModeLiveChainExecute && !e.cfg.EnableLiveChainExecution {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "LIVE_CHAIN_FLAG_DISABLED")
	}
	if e.cfg.Mode == types.ModeLiveExchangeExecute && !e.cfg.EnableLiveExchangeExecution {
		d.Allowed = false
		d.ReasonCodes = append(d.ReasonCodes, "LIVE_EXCHANGE_FLAG_DISABLED")
	}

	if d.Allowed {
		e.lastAction = now
		e.dailyUsed += amountDOGE
		d.ReasonCodes = append(d.ReasonCodes, "POLICY_ALLOW")
	}
	return d
}

func isWhitelisted(destination string, addresses, buckets []string) bool {
	d := strings.TrimSpace(destination)
	if d == "" {
		return false
	}
	for _, a := range addresses {
		if a == d {
			return true
		}
	}
	for _, b := range buckets {
		if b == d {
			return true
		}
	}
	return false
}
