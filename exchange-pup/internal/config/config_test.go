package config

import (
"os"
"path/filepath"
"testing"

"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func TestLoadDefaultsAndEnvOverride(t *testing.T) {
dir := t.TempDir()
p := filepath.Join(dir, "cfg.yaml")
if err := os.WriteFile(p, []byte("mode: recommend_only\nsqlite_path: /tmp/a.db\nnode_rpc:\n  url: http://127.0.0.1:22555\npolicy:\n  whitelisted_buckets: [treasury]\n  max_single_action_amount_doge: 1\n  max_daily_amount_doge: 2\n  min_confidence: 0.5\n"), 0o600); err != nil {
t.Fatal(err)
}
t.Setenv("MODE", "paper_trade")
cfg, err := Load(p)
if err != nil {
t.Fatal(err)
}
if cfg.Mode != types.ModePaperTrade {
t.Fatalf("expected paper_trade got %s", cfg.Mode)
}
if cfg.Redacted()["mode"] != types.ModePaperTrade {
t.Fatalf("expected redacted mode")
}
}

func TestValidateRejectsLiveWithoutFlag(t *testing.T) {
cfg := defaultConfig()
cfg.Mode = types.ModeLiveChainExecute
cfg.EnableLiveChainExecution = false
if err := cfg.Validate(); err == nil {
t.Fatal("expected error")
}
}
