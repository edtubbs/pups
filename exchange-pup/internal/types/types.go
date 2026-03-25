package types

import "time"

type Mode string

const (
	ModeRecommendOnly       Mode = "recommend_only"
	ModePaperTrade          Mode = "paper_trade"
	ModeLiveChainExecute    Mode = "live_chain_execute"
	ModeLiveExchangeExecute Mode = "live_exchange_execute"
)

type Action string

const (
	ActionHold           Action = "HOLD"
	ActionBuyNow         Action = "BUY_NOW"
	ActionSellNow        Action = "SELL_NOW"
	ActionWaitNBlocks    Action = "WAIT_N_BLOCKS"
	ActionRebalance      Action = "REBALANCE"
	ActionMoveToExchange Action = "MOVE_TO_EXCHANGE"
	ActionMoveToTreasury Action = "MOVE_TO_TREASURY"
)

type Recommendation struct {
	SignalID       string         `json:"signal_id"`
	Action         Action         `json:"action"`
	RankedActions  []ScoredAction `json:"ranked_actions"`
	Confidence     float64        `json:"confidence"`
	ExpectedEdge   float64        `json:"expected_edge"`
	Horizon        string         `json:"horizon"`
	ReasonCodes    []string       `json:"reason_codes"`
	ReasonSummary  string         `json:"reason_summary"`
	ModelVersion   string         `json:"model_version"`
	FeatureSchema  string         `json:"feature_schema_version"`
	Timestamp      time.Time      `json:"timestamp"`
	PolicyDecision PolicyDecision `json:"policy_decision"`
}

type ScoredAction struct {
	Action      Action  `json:"action"`
	Score       float64 `json:"score"`
	Probability float64 `json:"probability"`
}

type FeatureVector struct {
	SchemaVersion string             `json:"schema_version"`
	Symbol        string             `json:"symbol"`
	Timestamp     time.Time          `json:"timestamp"`
	Values        map[string]float64 `json:"values"`
	Context       map[string]string  `json:"context"`
}

type MarketTick struct {
	Exchange  string    `json:"exchange"`
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	Volume    float64   `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}

type Candle struct {
	Exchange  string    `json:"exchange"`
	Symbol    string    `json:"symbol"`
	Interval  string    `json:"interval"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	OpenTime  time.Time `json:"open_time"`
	CloseTime time.Time `json:"close_time"`
}

type BookLevel struct {
	Price  float64 `json:"price"`
	Amount float64 `json:"amount"`
}

type OrderBook struct {
	Exchange  string      `json:"exchange"`
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Timestamp time.Time   `json:"timestamp"`
}

type NodeSnapshot struct {
	Chain                  string    `json:"chain"`
	Blocks                 int64     `json:"blocks"`
	Headers                int64     `json:"headers"`
	MempoolBytes           int64     `json:"mempool_bytes"`
	MempoolTxCount         int64     `json:"mempool_tx_count"`
	WalletBalance          float64   `json:"wallet_balance"`
	RecentWalletTxCount    int       `json:"recent_wallet_tx_count"`
	RecentBlockIntervalSec float64   `json:"recent_block_interval_sec"`
	Timestamp              time.Time `json:"timestamp"`
}

type PolicyDecision struct {
	Allowed          bool     `json:"allowed"`
	Mode             Mode     `json:"mode"`
	ReasonCodes      []string `json:"reason_codes"`
	RequiresApproval bool     `json:"requires_approval"`
	DryRun           bool     `json:"dry_run"`
}
