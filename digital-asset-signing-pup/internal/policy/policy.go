package policy

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/attestation"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
)

type EvaluationInput struct {
	ProviderID      string               `json:"provider_id"`
	DeviceID        string               `json:"device_id"`
	DeviceClass     string               `json:"device_class"`
	ManifestDigest  string               `json:"manifest_digest"`
	ArtifactDigest  string               `json:"artifact_digest"`
	EntitlementOK   bool                 `json:"entitlement_ok"`
	Revoked         bool                 `json:"revoked"`
	Attestation     attestation.Decision `json:"attestation"`
	RequestedAction string               `json:"requested_action"`
	OfflineMode     bool                 `json:"offline_mode"`
	EvidencePresent bool                 `json:"evidence_present"`
	Metadata        map[string]string    `json:"metadata"`
	Now             time.Time            `json:"now"`
}

type Evaluation struct {
	Allow   bool     `json:"allow"`
	Reasons []string `json:"reasons"`
}

type Engine struct {
	cfg config.PolicyConfig
	log *logging.Logger
}

func NewEngine(cfg config.PolicyConfig, log *logging.Logger) *Engine {
	return &Engine{cfg: cfg, log: log}
}

func (e *Engine) SetKillSwitch(on bool) { e.cfg.KillSwitch = on }

func (e *Engine) Evaluate(_ context.Context, in EvaluationInput) Evaluation {
	reasons := []string{}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	if e.cfg.KillSwitch {
		reasons = append(reasons, "global kill switch enabled")
		return Evaluation{Allow: false, Reasons: reasons}
	}
	if e.cfg.BlockUnauthenticatedExec && in.RequestedAction == "execute" && !in.EvidencePresent {
		reasons = append(reasons, "execution blocked without attestation evidence")
	}
	if len(e.cfg.ProviderAllowlist) > 0 && !slices.Contains(e.cfg.ProviderAllowlist, in.ProviderID) {
		reasons = append(reasons, "provider not allowlisted")
	}
	if len(e.cfg.DeviceClassAllowlist) > 0 && !slices.Contains(e.cfg.DeviceClassAllowlist, in.DeviceClass) {
		reasons = append(reasons, "device class not allowlisted")
	}
	if e.cfg.RequireRevocationChecking && in.Revoked {
		reasons = append(reasons, "revocation present")
	}
	if in.Attestation.State == attestation.Untrusted {
		reasons = append(reasons, "attestation untrusted")
	}
	if in.Attestation.State == attestation.Stale && !e.cfg.AllowStaleForReadOnly {
		reasons = append(reasons, "stale attestation denied")
	}
	if !in.EntitlementOK {
		reasons = append(reasons, "entitlement not valid")
	}
	if in.ManifestDigest == "" || in.ArtifactDigest == "" {
		reasons = append(reasons, "missing manifest or artifact digest")
	}
	if in.OfflineMode && !e.cfg.OfflineAllowEntitlement && !in.EntitlementOK {
		reasons = append(reasons, "offline entitlement check failed")
	}
	allow := len(reasons) == 0
	e.log.Info("policy evaluation", "allow", allow, "provider_id", in.ProviderID, "device_id", in.DeviceID)
	if !allow {
		e.log.Warn("policy denied", "reasons", fmt.Sprintf("%v", reasons))
	}
	return Evaluation{Allow: allow, Reasons: reasons}
}
