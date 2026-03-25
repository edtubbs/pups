package model

import (
	"context"
	"errors"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type FakeEngine struct {
	md     Metadata
	loaded bool
}

func NewFake() *FakeEngine { return &FakeEngine{} }

func (f *FakeEngine) LoadModel(_ string, metadata Metadata) error {
	f.md = metadata
	f.loaded = true
	return nil
}

func (f *FakeEngine) Predict(_ context.Context, fv types.FeatureVector) ([]types.ScoredAction, float64, error) {
	if !f.loaded {
		return nil, 0, errors.New("model not loaded")
	}
	m := fv.Values
	trend := m["ret_5"] + m["ema_fast_minus_slow"] + m["macd"]
	vol := m["volatility_5"]
	scores := map[types.Action]float64{
		types.ActionHold:           0.3,
		types.ActionWaitNBlocks:    0.2,
		types.ActionBuyNow:         0.2 + max(trend, 0),
		types.ActionSellNow:        0.2 + max(-trend, 0),
		types.ActionMoveToExchange: 0.1 + max(-vol, 0),
		types.ActionMoveToTreasury: 0.1 + max(vol, 0),
		types.ActionRebalance:      0.15 + abs(trend)*0.2,
	}
	ranked := Rank(scores)
	conf := ranked[0].Probability
	return ranked, conf, nil
}

func (f *FakeEngine) Metadata() Metadata { return f.md }
func (f *FakeEngine) Backend() string    { return "fake" }

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
