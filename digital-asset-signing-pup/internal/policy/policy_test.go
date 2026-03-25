package policy

import (
	"context"
	"testing"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/attestation"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
)

func TestPolicyDenyOnRevocation(t *testing.T) {
	e := NewEngine(config.Default().Policy, logging.New("error"))
	out := e.Evaluate(context.Background(), EvaluationInput{ProviderID: "p", DeviceID: "d", ManifestDigest: "m", ArtifactDigest: "a", EntitlementOK: true, Revoked: true, EvidencePresent: true, Attestation: attestation.Decision{State: attestation.Trusted}})
	if out.Allow {
		t.Fatalf("expected deny on revocation")
	}
}
