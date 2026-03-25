package features

import (
	"math"
	"sort"
	"time"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func BuildFeatureVector(schemaVersion string, symbol string, candles []types.Candle, tick types.MarketTick, book types.OrderBook, node types.NodeSnapshot) types.FeatureVector {
	now := time.Now().UTC()
	closes := extract(candles, func(c types.Candle) float64 { return c.Close })
	volumes := extract(candles, func(c types.Candle) float64 { return c.Volume })
	highs := extract(candles, func(c types.Candle) float64 { return c.High })
	lows := extract(candles, func(c types.Candle) float64 { return c.Low })

	ret1 := ret(closes, 1)
	ret5 := ret(closes, 5)
	ret20 := ret(closes, 20)
	vol5 := stdevWindow(closes, 5)
	vol20 := stdevWindow(closes, 20)
	emaFast := ema(closes, 12)
	emaSlow := ema(closes, 26)
	sma20 := sma(closes, 20)
	rsi14 := rsi(closes, 14)
	macdVal := emaFast - emaSlow
	signal := ema([]float64{macdVal}, 9)
	bbMiddle := sma20
	bbStd := stdevWindow(closes, 20)
	bbUpper := bbMiddle + (2 * bbStd)
	bbLower := bbMiddle - (2 * bbStd)
	bbPos := 0.5
	if bbUpper > bbLower {
		bbPos = (tick.Price - bbLower) / (bbUpper - bbLower)
	}
	atr14 := atr(highs, lows, closes, 14)
	obvVal := obv(closes, volumes)
	spread := 0.0
	if tick.Ask > 0 {
		spread = (tick.Ask - tick.Bid) / tick.Ask
	}
	midMove := ret([]float64{tick.Bid, tick.Price}, 1)
	slippageProxy := bookSlippageProxy(book)

	values := map[string]float64{
		"ret_1":                  ret1,
		"ret_5":                  ret5,
		"ret_20":                 ret20,
		"volatility_5":           vol5,
		"volatility_20":          vol20,
		"spread":                 spread,
		"mid_move":               midMove,
		"slippage_proxy":         slippageProxy,
		"rsi_14":                 rsi14,
		"macd":                   macdVal,
		"macd_signal":            signal,
		"ema_fast":               emaFast,
		"ema_slow":               emaSlow,
		"ema_fast_minus_slow":    emaFast - emaSlow,
		"sma_20":                 sma20,
		"bb_position":            bbPos,
		"atr_14":                 atr14,
		"obv":                    obvVal,
		"node_blocks":            float64(node.Blocks),
		"node_mempool_count":     float64(node.MempoolTxCount),
		"node_mempool_bytes":     float64(node.MempoolBytes),
		"wallet_balance":         node.WalletBalance,
		"wallet_recent_tx_count": float64(node.RecentWalletTxCount),
		"block_interval_seconds": node.RecentBlockIntervalSec,
		"hour_of_day":            float64(now.Hour()),
		"day_of_week":            float64(now.Weekday()),
		"rolling_horizon_marker": float64(now.Unix()%3600) / 3600,
	}

	context := map[string]string{
		"node_chain":             node.Chain,
		"target_bucket_id":       "treasury",
		"symbol":                 symbol,
		"feature_schema_version": schemaVersion,
	}

	return types.FeatureVector{SchemaVersion: schemaVersion, Symbol: symbol, Timestamp: now, Values: values, Context: context}
}

func extract(c []types.Candle, fn func(types.Candle) float64) []float64 {
	out := make([]float64, 0, len(c))
	for _, v := range c {
		out = append(out, fn(v))
	}
	return out
}

func ret(v []float64, w int) float64 {
	n := len(v)
	if n == 0 || n <= w {
		return 0
	}
	a := v[n-1-w]
	if a == 0 {
		return 0
	}
	return (v[n-1] - a) / a
}

func sma(v []float64, w int) float64 {
	if len(v) == 0 {
		return 0
	}
	if w <= 0 || w > len(v) {
		w = len(v)
	}
	s := 0.0
	for _, x := range v[len(v)-w:] {
		s += x
	}
	return s / float64(w)
}

func ema(v []float64, w int) float64 {
	if len(v) == 0 {
		return 0
	}
	if w <= 0 {
		return v[len(v)-1]
	}
	alpha := 2 / (float64(w) + 1)
	e := v[0]
	for i := 1; i < len(v); i++ {
		e = alpha*v[i] + (1-alpha)*e
	}
	return e
}

func stdevWindow(v []float64, w int) float64 {
	if len(v) == 0 {
		return 0
	}
	if w <= 0 || w > len(v) {
		w = len(v)
	}
	slice := v[len(v)-w:]
	m := sma(slice, len(slice))
	s := 0.0
	for _, x := range slice {
		d := x - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(slice)))
}

func rsi(v []float64, p int) float64 {
	if len(v) < p+1 {
		return 50
	}
	gain, loss := 0.0, 0.0
	for i := len(v) - p; i < len(v); i++ {
		d := v[i] - v[i-1]
		if d > 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	if loss == 0 {
		return 100
	}
	rs := gain / loss
	return 100 - (100 / (1 + rs))
}

func atr(highs, lows, closes []float64, p int) float64 {
	n := len(closes)
	if n == 0 || len(highs) != n || len(lows) != n {
		return 0
	}
	trs := make([]float64, 0, n)
	prevClose := closes[0]
	for i := 0; i < n; i++ {
		tr := math.Max(highs[i]-lows[i], math.Max(math.Abs(highs[i]-prevClose), math.Abs(lows[i]-prevClose)))
		trs = append(trs, tr)
		prevClose = closes[i]
	}
	return sma(trs, p)
}

func obv(closes, volumes []float64) float64 {
	n := len(closes)
	if n == 0 || len(volumes) != n {
		return 0
	}
	out := 0.0
	for i := 1; i < n; i++ {
		switch {
		case closes[i] > closes[i-1]:
			out += volumes[i]
		case closes[i] < closes[i-1]:
			out -= volumes[i]
		}
	}
	return out
}

func bookSlippageProxy(book types.OrderBook) float64 {
	totalBid := 0.0
	totalAsk := 0.0
	for _, b := range book.Bids {
		totalBid += b.Amount
	}
	for _, a := range book.Asks {
		totalAsk += a.Amount
	}
	t := totalBid + totalAsk
	if t == 0 {
		return 1
	}
	imbalance := math.Abs(totalBid-totalAsk) / t
	levels := append(append([]types.BookLevel{}, book.Bids...), book.Asks...)
	sort.Slice(levels, func(i, j int) bool { return levels[i].Price < levels[j].Price })
	return imbalance
}
