package blockchain

import (
	"context"
	"testing"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
)

func TestAnchorEncodeCreateVerify(t *testing.T) {
	c := NewClient(config.BlockchainConfig{MockMode: true})
	s := NewService(c, true)
	rec := s.EncodeAnchor("manifest", "abc", "ref", "provider")
	if rec.ID == "" {
		t.Fatalf("expected ID")
	}
	created, err := s.CreateAnchor(context.Background(), rec)
	if err != nil {
		t.Fatalf("create anchor: %v", err)
	}
	ok, _ := s.VerifyAnchor(context.Background(), created)
	if !ok {
		t.Fatalf("expected verified anchor")
	}
}
