package execution

import (
	"context"
	"path/filepath"
	"testing"

	"log/slog"

	"github.com/edtubbs/pups/exchange-pup/internal/config"
	"github.com/edtubbs/pups/exchange-pup/internal/storage"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func TestPaperExecute(t *testing.T) {
	db := filepath.Join(t.TempDir(), "paper.db")
	s, err := storage.Open(db, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	exec := NewPaper(config.Config{PaperTrading: config.PaperTradingConfig{AssumedFeeBps: 10, AssumedSlippageBps: 20}}, s)
	res, err := exec.Execute(context.Background(), types.Recommendation{SignalID: "s1"}, "DOGEUSDT", 0.2, 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "simulated" {
		t.Fatal("expected simulated")
	}
}
