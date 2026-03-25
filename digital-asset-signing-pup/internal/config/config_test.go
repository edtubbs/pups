package config

import "testing"

func TestValidateRejectsUnsafePolicy(t *testing.T) {
	cfg := Default()
	cfg.Policy.BlockUnauthenticatedExec = false
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateAcceptsDefault(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid default config: %v", err)
	}
}
