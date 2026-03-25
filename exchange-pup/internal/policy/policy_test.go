package policy

import (
	"testing"

	"github.com/edtubbs/pups/exchange-pup/internal/config"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func TestEvaluateBlocksRecommendOnly(t *testing.T) {
	cfg := config.Config{
		Mode:                      types.ModeRecommendOnly,
		RecommendOnlyMasterSwitch: true,
		DryRun:                    true,
		Policy: config.PolicyConfig{
			WhitelistedBuckets:        []string{"treasury"},
			MaxSingleActionAmountDOGE: 100,
			MaxDailyAmountDOGE:        1000,
			MinConfidence:             0.5,
			MinExpectedEdge:           -1,
			MaxSlippageBps:            100,
			MinLiquidity:              10,
		},
	}
	e := New(cfg)
	rec := types.Recommendation{Confidence: 0.7, ExpectedEdge: 0.1}
	d := e.Evaluate(rec, "treasury", 50, true, true, 1000, 10)
	if d.Allowed {
		t.Fatal("recommend_only should not allow execution")
	}
}

func TestEvaluateWhitelisting(t *testing.T) {
	cfg := config.Config{
		Mode: types.ModePaperTrade,
		Policy: config.PolicyConfig{
			WhitelistedBuckets:        []string{"treasury"},
			MaxSingleActionAmountDOGE: 100,
			MaxDailyAmountDOGE:        1000,
			MinConfidence:             0.1,
			MinExpectedEdge:           -1,
			MaxSlippageBps:            100,
			MinLiquidity:              10,
		},
	}
	e := New(cfg)
	rec := types.Recommendation{Confidence: 0.7, ExpectedEdge: 0.1}
	d := e.Evaluate(rec, "random", 50, true, true, 1000, 10)
	if d.Allowed {
		t.Fatal("non-whitelisted destination should be blocked")
	}
}
