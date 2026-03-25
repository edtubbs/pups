package market

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBinanceKrakenAdapterInterface(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/ticker/bookTicker":
			_ = json.NewEncoder(w).Encode(map[string]any{"bidPrice": "0.2", "askPrice": "0.21"})
		case "/api/v3/klines":
			_ = json.NewEncoder(w).Encode([][]any{
				{float64(1700000000000), "0.19", "0.22", "0.18", "0.20", "1000", float64(1700000060000)},
			})
		case "/0/public/Ticker":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"XDGUSDT": map[string]any{"a": []string{"0.21"}, "b": []string{"0.2"}}},
			})
		case "/0/public/OHLC":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"XDGUSDT": [][]any{
						{float64(1700000000), "0.19", "0.22", "0.18", "0.20", "0.0", "1000"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewClient()
	client.BinanceBaseURL = ts.URL
	client.KrakenBaseURL = ts.URL
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := NewBinance(client).FetchSnapshot(ctx, "DOGEUSDT", "1m"); err != nil {
		t.Fatalf("binance snapshot failed: %v", err)
	}
	if _, err := NewKraken(client).FetchSnapshot(ctx, "DOGEUSDT", "1m"); err != nil {
		t.Fatalf("kraken snapshot failed: %v", err)
	}
}
