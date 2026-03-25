package strategy

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/edtubbs/pups/exchange-pup/internal/model"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func BuildRecommendation(ranked []types.ScoredAction, conf float64, edge float64, md model.Metadata, fv types.FeatureVector) types.Recommendation {
	a := types.ActionHold
	if len(ranked) > 0 {
		a = ranked[0].Action
	}
	reasons := reasonCodes(fv)
	r := types.Recommendation{
		SignalID:      fmt.Sprintf("sig_%d", time.Now().UnixNano()),
		Action:        a,
		RankedActions: ranked,
		Confidence:    conf,
		ExpectedEdge:  edge,
		Horizon:       md.Horizon,
		ReasonCodes:   reasons,
		ReasonSummary: strings.Join(reasons, ","),
		ModelVersion:  md.ModelVersion,
		FeatureSchema: fv.SchemaVersion,
		Timestamp:     time.Now().UTC(),
	}
	return r
}

func reasonCodes(fv types.FeatureVector) []string {
	codes := []string{"SCHEMA_OK"}
	if fv.Values["rsi_14"] > 70 {
		codes = append(codes, "RSI_OVERBOUGHT")
	} else if fv.Values["rsi_14"] < 30 {
		codes = append(codes, "RSI_OVERSOLD")
	}
	if fv.Values["ema_fast_minus_slow"] > 0 {
		codes = append(codes, "TREND_UP")
	} else {
		codes = append(codes, "TREND_DOWN")
	}
	if fv.Values["volatility_20"] > 0.02 {
		codes = append(codes, "VOL_HIGH")
	}
	sort.Strings(codes)
	return codes
}
