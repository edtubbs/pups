package attestation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
)

type TrustState string

const (
	Trusted            TrustState = "trusted"
	TrustedWithWarning TrustState = "trusted_with_warnings"
	Untrusted          TrustState = "untrusted"
	Stale              TrustState = "stale"
)

type Evidence struct {
	DeviceID      string            `json:"device_id"`
	ProviderNonce string            `json:"provider_nonce"`
	TPMBacked     bool              `json:"tpm_backed"`
	Quote         string            `json:"quote"`
	PCRs          map[string]string `json:"pcrs"`
	DeviceClass   string            `json:"device_class"`
	Firmware      string            `json:"firmware"`
	AppMeasure    string            `json:"app_measurement"`
	CapturedAt    time.Time         `json:"captured_at"`
}

type Decision struct {
	State      TrustState `json:"state"`
	Reasons    []string   `json:"reasons"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CapturedAt time.Time  `json:"captured_at"`
	Digest     string     `json:"digest"`
}

type Service struct {
	db  *storage.DB
	cfg config.AttestationConfig
	log *logging.Logger
}

func NewService(db *storage.DB, cfg config.AttestationConfig, log *logging.Logger) *Service {
	return &Service{db: db, cfg: cfg, log: log}
}

func (s *Service) VerifyAndStore(ctx context.Context, ev Evidence) (Decision, error) {
	if ev.DeviceID == "" {
		return Decision{}, fmt.Errorf("device_id required")
	}
	if ev.CapturedAt.IsZero() {
		ev.CapturedAt = time.Now().UTC()
	}
	reasons := make([]string, 0)
	state := Trusted
	if s.cfg.RequireTPM && !ev.TPMBacked {
		reasons = append(reasons, "TPM required but evidence is not TPM-backed")
		state = Untrusted
	}
	if !ev.TPMBacked && !s.cfg.AllowSoftwareFallback {
		reasons = append(reasons, "software attestation fallback disabled")
		state = Untrusted
	}
	if time.Since(ev.CapturedAt) > s.cfg.MaxAge {
		reasons = append(reasons, "attestation evidence is stale")
		if state == Trusted {
			state = Stale
		}
	}
	for idx, expected := range s.cfg.RequiredPCRs {
		if got := ev.PCRs[idx]; got != expected {
			reasons = append(reasons, fmt.Sprintf("PCR %s mismatch", idx))
			state = Untrusted
		}
	}
	if len(s.cfg.AllowedDeviceClasses) > 0 {
		ok := false
		for _, allowed := range s.cfg.AllowedDeviceClasses {
			if allowed == ev.DeviceClass {
				ok = true
				break
			}
		}
		if !ok {
			reasons = append(reasons, "device class not allowlisted")
			if state == Trusted {
				state = TrustedWithWarning
			}
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "all attestation checks passed")
	}
	raw, _ := json.Marshal(ev)
	digest := sha256.Sum256(raw)
	decision := Decision{
		State:      state,
		Reasons:    reasons,
		CapturedAt: ev.CapturedAt,
		ExpiresAt:  ev.CapturedAt.Add(s.cfg.MaxAge),
		Digest:     hex.EncodeToString(digest[:]),
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO attestations(device_id, evidence_json, trust_state, reasons_json, digest, captured_at, expires_at) VALUES(?,?,?,?,?,?,?)`,
		ev.DeviceID, string(raw), decision.State, mustJSON(reasons), decision.Digest, decision.CapturedAt.Unix(), decision.ExpiresAt.Unix())
	if err != nil {
		return Decision{}, err
	}
	s.log.Info("attestation decision", "device_id", ev.DeviceID, "state", decision.State)
	return decision, nil
}

func (s *Service) Latest(ctx context.Context, deviceID string) (Decision, error) {
	var state, reasonsJSON, digest string
	var captured, expires int64
	err := s.db.QueryRowContext(ctx, `SELECT trust_state, reasons_json, digest, captured_at, expires_at FROM attestations WHERE device_id = ? ORDER BY captured_at DESC LIMIT 1`, deviceID).
		Scan(&state, &reasonsJSON, &digest, &captured, &expires)
	if err != nil {
		if err == sql.ErrNoRows {
			return Decision{}, fmt.Errorf("no attestation found")
		}
		return Decision{}, err
	}
	var reasons []string
	_ = json.Unmarshal([]byte(reasonsJSON), &reasons)
	return Decision{State: TrustState(state), Reasons: reasons, Digest: digest, CapturedAt: time.Unix(captured, 0).UTC(), ExpiresAt: time.Unix(expires, 0).UTC()}, nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
