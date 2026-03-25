package features

import (
	"testing"
	"time"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func TestBuildFeatureVector(t *testing.T) {
	candles := make([]types.Candle, 0, 40)
	now := time.Now().UTC()
	for i := 0; i < 40; i++ {
		c := float64(0.1 + float64(i)*0.001)
		candles = append(candles, types.Candle{Close: c, Open: c - 0.001, High: c + 0.002, Low: c - 0.002, Volume: 100 + float64(i), OpenTime: now.Add(time.Duration(i) * time.Minute), CloseTime: now.Add(time.Duration(i+1) * time.Minute)})
	}
	tick := types.MarketTick{Price: 0.2, Bid: 0.199, Ask: 0.201, Volume: 1000, Timestamp: now}
	book := types.OrderBook{Bids: []types.BookLevel{{Price: 0.199, Amount: 1000}}, Asks: []types.BookLevel{{Price: 0.201, Amount: 1100}}}
	node := types.NodeSnapshot{Blocks: 10, MempoolTxCount: 5, WalletBalance: 1000, RecentWalletTxCount: 3, RecentBlockIntervalSec: 60}
	fv := BuildFeatureVector("v1", "DOGEUSDT", candles, tick, book, node)
	if fv.SchemaVersion != "v1" {
		t.Fatal("schema mismatch")
	}
	if len(fv.Values) < 10 {
		t.Fatalf("expected features got %d", len(fv.Values))
	}
}
