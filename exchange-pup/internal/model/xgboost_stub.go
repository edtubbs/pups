package model

import (
	"context"
	"fmt"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type XGBoostEngine struct {
	md Metadata
}

func NewXGBoost() *XGBoostEngine { return &XGBoostEngine{} }

func (x *XGBoostEngine) LoadModel(path string, metadata Metadata) error {
	if path == "" {
		return fmt.Errorf("xgboost model path required")
	}
	x.md = metadata
	return nil
}

func (x *XGBoostEngine) Predict(_ context.Context, _ types.FeatureVector) ([]types.ScoredAction, float64, error) {
	return nil, 0, fmt.Errorf("xgboost backend scaffolded but not linked in this build")
}

func (x *XGBoostEngine) Metadata() Metadata { return x.md }
func (x *XGBoostEngine) Backend() string    { return "xgboost" }
