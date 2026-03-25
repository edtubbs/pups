package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func TestStoreInsertAndQuerySignals(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(db, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	rec := types.Recommendation{
		SignalID:      "sig-1",
		Action:        types.ActionHold,
		Confidence:    0.9,
		ExpectedEdge:  0.1,
		Horizon:       "1h",
		ModelVersion:  "m1",
		FeatureSchema: "v1",
		Timestamp:     time.Now().UTC(),
		PolicyDecision: types.PolicyDecision{
			Allowed: false,
		},
	}
	if err := s.InsertSignal(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.LatestSignals(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SignalID != "sig-1" {
		t.Fatal("unexpected signals query result")
	}
}
