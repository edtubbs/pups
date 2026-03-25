package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type Snapshot struct {
	Tick      types.MarketTick
	Book      types.OrderBook
	Candles   []types.Candle
	FetchedAt time.Time
}

type Adapter interface {
	Name() string
	FetchSnapshot(ctx context.Context, symbol string, interval string) (Snapshot, error)
}

type Client struct {
	http           *http.Client
	BinanceBaseURL string
	KrakenBaseURL  string
}

func NewClient() *Client {
	return &Client{
		http:           &http.Client{Timeout: 8 * time.Second},
		BinanceBaseURL: "https://api.binance.com",
		KrakenBaseURL:  "https://api.kraken.com",
	}
}

type BinanceAdapter struct{ c *Client }
type KrakenAdapter struct{ c *Client }

func NewBinance(c *Client) *BinanceAdapter { return &BinanceAdapter{c: c} }
func NewKraken(c *Client) *KrakenAdapter   { return &KrakenAdapter{c: c} }

func (a *BinanceAdapter) Name() string { return "binance" }
func (a *BinanceAdapter) FetchSnapshot(ctx context.Context, symbol string, interval string) (Snapshot, error) {
	now := time.Now().UTC()
	tickURL := fmt.Sprintf("%s/api/v3/ticker/bookTicker?symbol=%s", a.c.BinanceBaseURL, symbol)
	var ticker struct {
		BidPrice string `json:"bidPrice"`
		AskPrice string `json:"askPrice"`
	}
	if err := a.c.getJSON(ctx, tickURL, &ticker); err != nil {
		return Snapshot{}, err
	}
	bid, _ := strconv.ParseFloat(ticker.BidPrice, 64)
	ask, _ := strconv.ParseFloat(ticker.AskPrice, 64)
	price := (bid + ask) / 2

	klineURL := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=200", a.c.BinanceBaseURL, symbol, interval)
	var klines [][]any
	if err := a.c.getJSON(ctx, klineURL, &klines); err != nil {
		return Snapshot{}, err
	}
	candles := make([]types.Candle, 0, len(klines))
	for _, k := range klines {
		if len(k) < 7 {
			continue
		}
		o, _ := strconv.ParseFloat(asString(k[1]), 64)
		h, _ := strconv.ParseFloat(asString(k[2]), 64)
		l, _ := strconv.ParseFloat(asString(k[3]), 64)
		c, _ := strconv.ParseFloat(asString(k[4]), 64)
		v, _ := strconv.ParseFloat(asString(k[5]), 64)
		openMS, _ := asInt64(k[0])
		closeMS, _ := asInt64(k[6])
		candles = append(candles, types.Candle{
			Exchange: "binance", Symbol: symbol, Interval: interval,
			Open: o, High: h, Low: l, Close: c, Volume: v,
			OpenTime: time.UnixMilli(openMS).UTC(), CloseTime: time.UnixMilli(closeMS).UTC(),
		})
	}
	book := types.OrderBook{Exchange: "binance", Symbol: symbol, Timestamp: now,
		Bids: []types.BookLevel{{Price: bid, Amount: 1000}}, Asks: []types.BookLevel{{Price: ask, Amount: 1000}}}
	tick := types.MarketTick{Exchange: "binance", Symbol: symbol, Price: price, Bid: bid, Ask: ask, Volume: lastVolume(candles), Timestamp: now}
	return Snapshot{Tick: tick, Book: book, Candles: candles, FetchedAt: now}, nil
}

func (a *KrakenAdapter) Name() string { return "kraken" }
func (a *KrakenAdapter) FetchSnapshot(ctx context.Context, symbol string, interval string) (Snapshot, error) {
	pair := "XDGUSD"
	if symbol == "DOGEUSDT" {
		pair = "XDGUSDT"
	}
	now := time.Now().UTC()
	tickerURL := fmt.Sprintf("%s/0/public/Ticker?pair=%s", a.c.KrakenBaseURL, pair)
	var ticker struct {
		Result map[string]struct {
			Ask []string `json:"a"`
			Bid []string `json:"b"`
		} `json:"result"`
	}
	if err := a.c.getJSON(ctx, tickerURL, &ticker); err != nil {
		return Snapshot{}, err
	}
	var bid, ask float64
	for _, v := range ticker.Result {
		if len(v.Bid) > 0 {
			bid, _ = strconv.ParseFloat(v.Bid[0], 64)
		}
		if len(v.Ask) > 0 {
			ask, _ = strconv.ParseFloat(v.Ask[0], 64)
		}
		break
	}
	price := (bid + ask) / 2

	ohlcURL := fmt.Sprintf("%s/0/public/OHLC?pair=%s&interval=1", a.c.KrakenBaseURL, pair)
	var ohlc struct {
		Result map[string][][]any `json:"result"`
	}
	if err := a.c.getJSON(ctx, ohlcURL, &ohlc); err != nil {
		return Snapshot{}, err
	}
	candles := make([]types.Candle, 0, 200)
	for k, rows := range ohlc.Result {
		if k == "last" {
			continue
		}
		for _, r := range rows {
			if len(r) < 7 {
				continue
			}
			ts, _ := asInt64(r[0])
			o, _ := strconv.ParseFloat(asString(r[1]), 64)
			h, _ := strconv.ParseFloat(asString(r[2]), 64)
			l, _ := strconv.ParseFloat(asString(r[3]), 64)
			c, _ := strconv.ParseFloat(asString(r[4]), 64)
			v, _ := strconv.ParseFloat(asString(r[6]), 64)
			ot := time.Unix(ts, 0).UTC()
			candles = append(candles, types.Candle{Exchange: "kraken", Symbol: symbol, Interval: interval, Open: o, High: h, Low: l, Close: c, Volume: v, OpenTime: ot, CloseTime: ot.Add(time.Minute)})
		}
		break
	}
	book := types.OrderBook{Exchange: "kraken", Symbol: symbol, Timestamp: now,
		Bids: []types.BookLevel{{Price: bid, Amount: 1000}}, Asks: []types.BookLevel{{Price: ask, Amount: 1000}}}
	tick := types.MarketTick{Exchange: "kraken", Symbol: symbol, Price: price, Bid: bid, Ask: ask, Volume: lastVolume(candles), Timestamp: now}
	return Snapshot{Tick: tick, Book: book, Candles: candles, FetchedAt: now}, nil
}

func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("http status %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
func asInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case json.Number:
		return t.Int64()
	default:
		return 0, fmt.Errorf("unsupported int type")
	}
}
func lastVolume(c []types.Candle) float64 {
	if len(c) == 0 {
		return 0
	}
	return c[len(c)-1].Volume
}
