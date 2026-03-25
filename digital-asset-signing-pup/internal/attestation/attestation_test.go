package attestation

import (
	"context"
	"testing"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
)

func TestAttestationPolicy(t *testing.T) {
	db, err := storage.Open(context.Background(), "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	s := NewService(db, config.AttestationConfig{RequireTPM: true, AllowSoftwareFallback: false, MaxAge: 5 * time.Minute, RequiredPCRs: map[string]string{"0": "ok"}}, logging.New("error"))

	ev := Evidence{DeviceID: "d1", TPMBacked: true, PCRs: map[string]string{"0": "ok"}, CapturedAt: time.Now().UTC()}
	dec, err := s.VerifyAndStore(context.Background(), ev)
	if err != nil || dec.State != Trusted {
		t.Fatalf("expected trusted decision, got %v err=%v", dec.State, err)
	}
}
