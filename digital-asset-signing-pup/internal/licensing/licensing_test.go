package licensing

import (
	"context"
	"testing"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
)

func TestEntitlementEvaluation(t *testing.T) {
	db, err := storage.Open(context.Background(), "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	s := NewService(db, true, logging.New("error"))
	_, err = s.Create(context.Background(), Entitlement{ID: "e1", ArtifactID: "a1", SubjectType: "user", SubjectID: "u1", Transferable: true})
	if err != nil {
		t.Fatalf("create entitlement: %v", err)
	}
	ok, reason, err := s.Evaluate(context.Background(), "e1", "u1", time.Now().UTC())
	if err != nil || !ok || reason != "ok" {
		t.Fatalf("expected entitlement ok, got ok=%v reason=%s err=%v", ok, reason, err)
	}
}
