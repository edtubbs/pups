package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

const CurrentFeatureSchemaVersion = "v1"

type Config struct {
	Mode                         types.Mode         `yaml:"mode"`
	EnableLiveChainExecution     bool               `yaml:"enable_live_chain_execution"`
	EnableLiveExchangeExecution  bool               `yaml:"enable_live_exchange_execution"`
	RecommendOnlyMasterSwitch    bool               `yaml:"recommend_only_master_switch"`
	PaperTradeOnlyMasterSwitch   bool               `yaml:"paper_trade_only_master_switch"`
	DryRun                       bool               `yaml:"dry_run"`
	KillSwitchInitialState       bool               `yaml:"kill_switch_initial_state"`
	RequireHumanApproval         bool               `yaml:"require_human_approval"`
	RequireHumanApprovalOverConf float64            `yaml:"require_human_approval_over_confidence"`
	HTTPBind                     string             `yaml:"http_bind"`
	MetricsBind                  string             `yaml:"metrics_bind"`
	SQLitePath                   string             `yaml:"sqlite_path"`
	ModelPath                    string             `yaml:"model_path"`
	ModelMetadataPath            string             `yaml:"model_metadata_path"`
	ModelBackend                 string             `yaml:"model_backend"`
	FeatureSchemaVersion         string             `yaml:"feature_schema_version"`
	NodeRPC                      NodeRPCConfig      `yaml:"node_rpc"`
	Market                       MarketConfig       `yaml:"market"`
	Policy                       PolicyConfig       `yaml:"policy"`
	PaperTrading                 PaperTradingConfig `yaml:"paper_trading"`
	Secrets                      SecretsConfig      `yaml:"secrets"`
}

type NodeRPCConfig struct {
	URL      string        `yaml:"url"`
	User     string        `yaml:"user"`
	Password string        `yaml:"password"`
	Timeout  time.Duration `yaml:"timeout"`
}

type MarketConfig struct {
	Symbols                  []string      `yaml:"symbols"`
	Exchanges                []string      `yaml:"exchanges"`
	CandleIntervals          []string      `yaml:"candle_intervals"`
	RESTPollInterval         time.Duration `yaml:"rest_poll_interval"`
	EnableWebsocketBinance   bool          `yaml:"enable_websocket_binance"`
	EnableWebsocketKraken    bool          `yaml:"enable_websocket_kraken"`
	MaxDataStaleness         time.Duration `yaml:"max_data_staleness"`
	RequireMinOrderbookDepth float64       `yaml:"require_min_orderbook_depth"`
}

type PolicyConfig struct {
	WhitelistedBuckets         []string          `yaml:"whitelisted_buckets"`
	WhitelistedAddresses       []string          `yaml:"whitelisted_addresses"`
	MaxSingleActionAmountDOGE  float64           `yaml:"max_single_action_amount_doge"`
	MaxDailyAmountDOGE         float64           `yaml:"max_daily_amount_doge"`
	Cooldown                   time.Duration     `yaml:"cooldown"`
	MinConfidence              float64           `yaml:"min_confidence"`
	MinExpectedEdge            float64           `yaml:"min_expected_edge"`
	MaxSlippageBps             float64           `yaml:"max_slippage_bps"`
	MinLiquidity               float64           `yaml:"min_liquidity"`
	AllowedDestinationByBucket map[string]string `yaml:"allowed_destination_by_bucket"`
}

type PaperTradingConfig struct {
	AssumedFeeBps      float64 `yaml:"assumed_fee_bps"`
	AssumedSlippageBps float64 `yaml:"assumed_slippage_bps"`
	StartingCashDOGE   float64 `yaml:"starting_cash_doge"`
}

type SecretsConfig struct {
	BinanceAPIKey    string `yaml:"binance_api_key"`
	BinanceAPISecret string `yaml:"binance_api_secret"`
	KrakenAPIKey     string `yaml:"kraken_api_key"`
	KrakenAPISecret  string `yaml:"kraken_api_secret"`
}

func Load(path string) (Config, error) {
	cfg := defaultConfig()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config: %w", err)
		}
	}
	applyEnvOverrides(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaultConfig() Config {
	return Config{
		Mode:                         types.ModeRecommendOnly,
		EnableLiveChainExecution:     false,
		EnableLiveExchangeExecution:  false,
		RecommendOnlyMasterSwitch:    true,
		PaperTradeOnlyMasterSwitch:   false,
		DryRun:                       true,
		KillSwitchInitialState:       false,
		RequireHumanApproval:         true,
		RequireHumanApprovalOverConf: 0.85,
		HTTPBind:                     "0.0.0.0:8099",
		MetricsBind:                  "0.0.0.0:9109",
		SQLitePath:                   "/storage/exchange-pup.db",
		ModelBackend:                 "fake",
		FeatureSchemaVersion:         CurrentFeatureSchemaVersion,
		NodeRPC: NodeRPCConfig{
			URL:     "http://127.0.0.1:22555",
			Timeout: 5 * time.Second,
		},
		Market: MarketConfig{
			Symbols:                  []string{"DOGEUSDT"},
			Exchanges:                []string{"binance", "kraken"},
			CandleIntervals:          []string{"1m", "5m", "15m"},
			RESTPollInterval:         10 * time.Second,
			EnableWebsocketBinance:   false,
			EnableWebsocketKraken:    false,
			MaxDataStaleness:         60 * time.Second,
			RequireMinOrderbookDepth: 1000,
		},
		Policy: PolicyConfig{
			WhitelistedBuckets:        []string{"exchange_deposit", "treasury", "hot_wallet", "cold_storage"},
			MaxSingleActionAmountDOGE: 500,
			MaxDailyAmountDOGE:        2000,
			Cooldown:                  2 * time.Minute,
			MinConfidence:             0.55,
			MinExpectedEdge:           0,
			MaxSlippageBps:            50,
			MinLiquidity:              1000,
			AllowedDestinationByBucket: map[string]string{
				"exchange_deposit": "exchange_deposit",
				"treasury":         "treasury",
			},
		},
		PaperTrading: PaperTradingConfig{
			AssumedFeeBps:      10,
			AssumedSlippageBps: 15,
			StartingCashDOGE:   10000,
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("MODE")); v != "" {
		cfg.Mode = types.Mode(v)
	}
	if v := strings.TrimSpace(os.Getenv("HTTP_BIND")); v != "" {
		cfg.HTTPBind = v
	}
	if v := strings.TrimSpace(os.Getenv("METRICS_BIND")); v != "" {
		cfg.MetricsBind = v
	}
	if v := strings.TrimSpace(os.Getenv("SQLITE_PATH")); v != "" {
		cfg.SQLitePath = v
	}
	if v := strings.TrimSpace(os.Getenv("CORE_RPC_URL")); v != "" {
		cfg.NodeRPC.URL = v
	}
	if v := strings.TrimSpace(os.Getenv("CORE_RPC_USER")); v != "" {
		cfg.NodeRPC.User = v
	}
	if v := strings.TrimSpace(os.Getenv("CORE_RPC_PASSWORD")); v != "" {
		cfg.NodeRPC.Password = v
	}
	if v := strings.TrimSpace(os.Getenv("BINANCE_API_KEY")); v != "" {
		cfg.Secrets.BinanceAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("BINANCE_API_SECRET")); v != "" {
		cfg.Secrets.BinanceAPISecret = v
	}
	if v := strings.TrimSpace(os.Getenv("KRAKEN_API_KEY")); v != "" {
		cfg.Secrets.KrakenAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("KRAKEN_API_SECRET")); v != "" {
		cfg.Secrets.KrakenAPISecret = v
	}
}

func (c Config) Validate() error {
	if c.Mode == "" {
		return errors.New("mode is required")
	}
	switch c.Mode {
	case types.ModeRecommendOnly, types.ModePaperTrade, types.ModeLiveChainExecute, types.ModeLiveExchangeExecute:
	default:
		return fmt.Errorf("unsupported mode: %s", c.Mode)
	}
	if c.SQLitePath == "" {
		return errors.New("sqlite_path is required")
	}
	if c.NodeRPC.URL == "" {
		return errors.New("node_rpc.url is required")
	}
	if c.FeatureSchemaVersion == "" {
		return errors.New("feature_schema_version is required")
	}
	if c.Mode == types.ModeLiveChainExecute && !c.EnableLiveChainExecution {
		return errors.New("live_chain_execute mode requires enable_live_chain_execution=true")
	}
	if c.Mode == types.ModeLiveExchangeExecute && !c.EnableLiveExchangeExecution {
		return errors.New("live_exchange_execute mode requires enable_live_exchange_execution=true")
	}
	if len(c.Policy.WhitelistedBuckets) == 0 && len(c.Policy.WhitelistedAddresses) == 0 {
		return errors.New("at least one whitelist bucket or address must be configured")
	}
	if c.Policy.MaxSingleActionAmountDOGE <= 0 || c.Policy.MaxDailyAmountDOGE <= 0 {
		return errors.New("policy amount limits must be > 0")
	}
	if c.Policy.MinConfidence < 0 || c.Policy.MinConfidence > 1 {
		return errors.New("policy min_confidence must be in [0,1]")
	}
	return nil
}

func (c Config) Redacted() map[string]any {
	return map[string]any{
		"mode":                           c.Mode,
		"enable_live_chain_execution":    c.EnableLiveChainExecution,
		"enable_live_exchange_execution": c.EnableLiveExchangeExecution,
		"recommend_only_master_switch":   c.RecommendOnlyMasterSwitch,
		"paper_trade_only_master_switch": c.PaperTradeOnlyMasterSwitch,
		"dry_run":                        c.DryRun,
		"kill_switch_initial_state":      c.KillSwitchInitialState,
		"require_human_approval":         c.RequireHumanApproval,
		"http_bind":                      c.HTTPBind,
		"metrics_bind":                   c.MetricsBind,
		"sqlite_path":                    c.SQLitePath,
		"model_path":                     c.ModelPath,
		"model_metadata_path":            c.ModelMetadataPath,
		"model_backend":                  c.ModelBackend,
		"feature_schema_version":         c.FeatureSchemaVersion,
		"node_rpc": map[string]any{
			"url":      c.NodeRPC.URL,
			"user":     redact(c.NodeRPC.User),
			"password": redact(c.NodeRPC.Password),
			"timeout":  c.NodeRPC.Timeout.String(),
		},
		"secrets": map[string]any{
			"binance_api_key":    redact(c.Secrets.BinanceAPIKey),
			"binance_api_secret": redact(c.Secrets.BinanceAPISecret),
			"kraken_api_key":     redact(c.Secrets.KrakenAPIKey),
			"kraken_api_secret":  redact(c.Secrets.KrakenAPISecret),
		},
	}
}

func redact(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "***REDACTED***"
}
