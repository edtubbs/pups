package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Runtime      RuntimeConfig      `json:"runtime" yaml:"runtime" toml:"runtime"`
	Blockchain   BlockchainConfig   `json:"blockchain" yaml:"blockchain" toml:"blockchain"`
	Storage      StorageConfig      `json:"storage" yaml:"storage" toml:"storage"`
	Trust        TrustConfig        `json:"trust" yaml:"trust" toml:"trust"`
	Attestation  AttestationConfig  `json:"attestation" yaml:"attestation" toml:"attestation"`
	Distribution DistributionConfig `json:"distribution" yaml:"distribution" toml:"distribution"`
	Policy       PolicyConfig       `json:"policy" yaml:"policy" toml:"policy"`
	Logging      LoggingConfig      `json:"logging" yaml:"logging" toml:"logging"`
}

type RuntimeConfig struct {
	HTTPBind                string `json:"http_bind" yaml:"http_bind" toml:"http_bind"`
	MetricsBind             string `json:"metrics_bind" yaml:"metrics_bind" toml:"metrics_bind"`
	OfflineVerificationMode bool   `json:"offline_verification_mode" yaml:"offline_verification_mode" toml:"offline_verification_mode"`
	KillSwitchInitialState  bool   `json:"kill_switch_initial_state" yaml:"kill_switch_initial_state" toml:"kill_switch_initial_state"`
}

type BlockchainConfig struct {
	RPCURL         string        `json:"rpc_url" yaml:"rpc_url" toml:"rpc_url"`
	RPCUser        string        `json:"rpc_user" yaml:"rpc_user" toml:"rpc_user"`
	RPCPass        string        `json:"rpc_pass" yaml:"rpc_pass" toml:"rpc_pass"`
	RPCTimeout     time.Duration `json:"rpc_timeout" yaml:"rpc_timeout" toml:"rpc_timeout"`
	MockMode       bool          `json:"mock_mode" yaml:"mock_mode" toml:"mock_mode"`
	WalletLabel    string        `json:"wallet_label" yaml:"wallet_label" toml:"wallet_label"`
	OPReturnPrefix string        `json:"op_return_prefix" yaml:"op_return_prefix" toml:"op_return_prefix"`
}

type StorageConfig struct {
	SQLitePath string `json:"sqlite_path" yaml:"sqlite_path" toml:"sqlite_path"`
}

type TrustConfig struct {
	TrustedProviderKeys map[string]string `json:"trusted_provider_keys" yaml:"trusted_provider_keys" toml:"trusted_provider_keys"`
	TrustedDevKeys      map[string]string `json:"trusted_dev_keys" yaml:"trusted_dev_keys" toml:"trusted_dev_keys"`
	MinProvenanceSigs   int               `json:"min_provenance_signatures" yaml:"min_provenance_signatures" toml:"min_provenance_signatures"`
}

type AttestationConfig struct {
	MockMode              bool              `json:"mock_mode" yaml:"mock_mode" toml:"mock_mode"`
	RequireTPM            bool              `json:"require_tpm" yaml:"require_tpm" toml:"require_tpm"`
	AllowSoftwareFallback bool              `json:"allow_software_fallback" yaml:"allow_software_fallback" toml:"allow_software_fallback"`
	MaxAge                time.Duration     `json:"max_age" yaml:"max_age" toml:"max_age"`
	RequiredPCRs          map[string]string `json:"required_pcrs" yaml:"required_pcrs" toml:"required_pcrs"`
	AllowedDeviceClasses  []string          `json:"allowed_device_classes" yaml:"allowed_device_classes" toml:"allowed_device_classes"`
}

type DistributionConfig struct {
	CacheDir       string   `json:"cache_dir" yaml:"cache_dir" toml:"cache_dir"`
	BackendURLs    []string `json:"backend_urls" yaml:"backend_urls" toml:"backend_urls"`
	RetryCount     int      `json:"retry_count" yaml:"retry_count" toml:"retry_count"`
	TimeoutSeconds int      `json:"timeout_seconds" yaml:"timeout_seconds" toml:"timeout_seconds"`
}

type PolicyConfig struct {
	LiveAnchoringEnabled      bool     `json:"live_anchoring_enabled" yaml:"live_anchoring_enabled" toml:"live_anchoring_enabled"`
	TransfersEnabled          bool     `json:"transfers_enabled" yaml:"transfers_enabled" toml:"transfers_enabled"`
	KillSwitch                bool     `json:"kill_switch" yaml:"kill_switch" toml:"kill_switch"`
	OfflineAllowEntitlement   bool     `json:"offline_allow_entitlement" yaml:"offline_allow_entitlement" toml:"offline_allow_entitlement"`
	ProviderAllowlist         []string `json:"provider_allowlist" yaml:"provider_allowlist" toml:"provider_allowlist"`
	DeviceClassAllowlist      []string `json:"device_class_allowlist" yaml:"device_class_allowlist" toml:"device_class_allowlist"`
	AllowStaleForReadOnly     bool     `json:"allow_stale_for_read_only" yaml:"allow_stale_for_read_only" toml:"allow_stale_for_read_only"`
	BlockUnauthenticatedExec  bool     `json:"block_unauthenticated_exec" yaml:"block_unauthenticated_exec" toml:"block_unauthenticated_exec"`
	RequireRevocationChecking bool     `json:"require_revocation_checking" yaml:"require_revocation_checking" toml:"require_revocation_checking"`
}

type LoggingConfig struct {
	Level string `json:"level" yaml:"level" toml:"level"`
}

func Default() Config {
	return Config{
		Runtime: RuntimeConfig{
			HTTPBind:                "0.0.0.0:8108",
			MetricsBind:             "0.0.0.0:9108",
			OfflineVerificationMode: true,
			KillSwitchInitialState:  false,
		},
		Blockchain: BlockchainConfig{
			RPCURL:         "http://127.0.0.1:22555",
			RPCTimeout:     8 * time.Second,
			MockMode:       true,
			WalletLabel:    "digital-asset-signing-pup",
			OPReturnPrefix: "DASP1",
		},
		Storage: StorageConfig{SQLitePath: "/storage/digital-asset-signing-pup.db"},
		Trust: TrustConfig{
			TrustedProviderKeys: map[string]string{},
			TrustedDevKeys:      map[string]string{},
			MinProvenanceSigs:   1,
		},
		Attestation: AttestationConfig{
			MockMode:              true,
			RequireTPM:            false,
			AllowSoftwareFallback: true,
			MaxAge:                10 * time.Minute,
			RequiredPCRs:          map[string]string{},
			AllowedDeviceClasses:  []string{},
		},
		Distribution: DistributionConfig{
			CacheDir:       "/storage/cache",
			BackendURLs:    []string{},
			RetryCount:     2,
			TimeoutSeconds: 15,
		},
		Policy: PolicyConfig{
			LiveAnchoringEnabled:      false,
			TransfersEnabled:          false,
			KillSwitch:                false,
			OfflineAllowEntitlement:   true,
			ProviderAllowlist:         []string{},
			DeviceClassAllowlist:      []string{},
			AllowStaleForReadOnly:     false,
			BlockUnauthenticatedExec:  true,
			RequireRevocationChecking: true,
		},
		Logging: LoggingConfig{Level: "info"},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = "/storage/config.yaml"
	}
	if b, err := os.ReadFile(path); err == nil {
		switch ext := strings.ToLower(filepath.Ext(path)); ext {
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse yaml: %w", err)
			}
		case ".toml":
			if err := toml.Unmarshal(b, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse toml: %w", err)
			}
		case ".json":
			if err := json.Unmarshal(b, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse json: %w", err)
			}
		default:
			return Config{}, fmt.Errorf("unsupported config extension: %s", ext)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	overrideFromEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("HTTP_BIND"); v != "" {
		cfg.Runtime.HTTPBind = v
	}
	if v := os.Getenv("METRICS_BIND"); v != "" {
		cfg.Runtime.MetricsBind = v
	}
	if v := os.Getenv("DOGECOIN_RPC_URL"); v != "" {
		cfg.Blockchain.RPCURL = v
	}
	if v := os.Getenv("DOGECOIN_RPC_USER"); v != "" {
		cfg.Blockchain.RPCUser = v
	}
	if v := os.Getenv("DOGECOIN_RPC_PASS"); v != "" {
		cfg.Blockchain.RPCPass = v
	}
	if v := os.Getenv("SQLITE_PATH"); v != "" {
		cfg.Storage.SQLitePath = v
	}
	if v := os.Getenv("OFFLINE_VERIFICATION"); v != "" {
		cfg.Runtime.OfflineVerificationMode = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("LIVE_ANCHORING"); v != "" {
		cfg.Policy.LiveAnchoringEnabled = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("ENABLE_TRANSFERS"); v != "" {
		cfg.Policy.TransfersEnabled = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("RPC_TIMEOUT_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.Blockchain.RPCTimeout = time.Duration(secs) * time.Second
		}
	}
}

func (c Config) Validate() error {
	if c.Runtime.HTTPBind == "" {
		return errors.New("runtime.http_bind is required")
	}
	if c.Runtime.MetricsBind == "" {
		return errors.New("runtime.metrics_bind is required")
	}
	if c.Storage.SQLitePath == "" {
		return errors.New("storage.sqlite_path is required")
	}
	if c.Blockchain.RPCURL == "" && !c.Blockchain.MockMode {
		return errors.New("blockchain.rpc_url is required unless mock_mode=true")
	}
	if c.Attestation.MaxAge <= 0 {
		return errors.New("attestation.max_age must be > 0")
	}
	if c.Trust.MinProvenanceSigs < 1 {
		return errors.New("trust.min_provenance_signatures must be >= 1")
	}
	if c.Policy.BlockUnauthenticatedExec == false {
		return errors.New("policy.block_unauthenticated_exec must stay enabled")
	}
	return nil
}
