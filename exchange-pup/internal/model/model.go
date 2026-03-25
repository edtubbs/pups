package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type Metadata struct {
	ModelVersion         string         `json:"model_version"`
	FeatureSchemaVersion string         `json:"feature_schema_version"`
	TargetLabels         []types.Action `json:"target_labels"`
	Horizon              string         `json:"horizon"`
	Calibration          map[string]any `json:"calibration,omitempty"`
}

type Engine interface {
	LoadModel(path string, metadata Metadata) error
	Predict(ctx context.Context, fv types.FeatureVector) ([]types.ScoredAction, float64, error)
	Metadata() Metadata
	Backend() string
}

func LoadMetadata(path string) (Metadata, error) {
	var m Metadata
	b, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("read metadata: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse metadata: %w", err)
	}
	if m.ModelVersion == "" || m.FeatureSchemaVersion == "" {
		return m, fmt.Errorf("metadata missing model_version or feature_schema_version")
	}
	if len(m.TargetLabels) == 0 {
		m.TargetLabels = []types.Action{types.ActionHold, types.ActionWaitNBlocks, types.ActionBuyNow, types.ActionSellNow, types.ActionMoveToExchange, types.ActionMoveToTreasury, types.ActionRebalance}
	}
	if m.Horizon == "" {
		m.Horizon = "short"
	}
	return m, nil
}

func ValidateSchema(expected, got string) error {
	if expected != got {
		return fmt.Errorf("feature schema mismatch expected=%s got=%s", expected, got)
	}
	return nil
}

func Rank(actions map[types.Action]float64) []types.ScoredAction {
	total := 0.0
	for _, v := range actions {
		if v > 0 {
			total += v
		}
	}
	out := make([]types.ScoredAction, 0, len(actions))
	for k, v := range actions {
		prob := 0.0
		if total > 0 && v > 0 {
			prob = v / total
		}
		out = append(out, types.ScoredAction{Action: k, Score: v, Probability: prob})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
