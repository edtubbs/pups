package trust

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/artifact"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
)

type Service struct {
	db  *storage.DB
	log *logging.Logger
}

type VerificationResult struct {
	Valid   bool     `json:"valid"`
	Reasons []string `json:"reasons"`
	Digest  string   `json:"digest"`
}

type DSSEEnvelope struct {
	PayloadType string   `json:"payloadType"`
	Payload     string   `json:"payload"`
	Signatures  []DSSSig `json:"signatures"`
}

type DSSSig struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

func NewService(db *storage.DB, log *logging.Logger) *Service { return &Service{db: db, log: log} }

func (s *Service) VerifyManifest(_ context.Context, m artifact.Manifest, providerPubKey string) VerificationResult {
	reasons := []string{}
	if m.SchemaVersion == "" {
		reasons = append(reasons, "missing schema version")
	}
	if m.ArtifactDigest == "" {
		reasons = append(reasons, "missing artifact digest")
	}
	if m.ProviderIdentity == "" {
		reasons = append(reasons, "missing provider identity")
	}
	digest := hashManifest(m)
	if providerPubKey == "" {
		reasons = append(reasons, "missing provider public key")
	} else if !verifyDetached(providerPubKey, digest, m.Signature) {
		reasons = append(reasons, "invalid provider signature")
	}
	valid := len(reasons) == 0
	if valid {
		reasons = append(reasons, "manifest signature and schema checks passed")
	}
	return VerificationResult{Valid: valid, Reasons: reasons, Digest: digest}
}

func (s *Service) VerifyInTotoEnvelope(ctx context.Context, env DSSEEnvelope, trustedKeys map[string]string, minSignatures int) VerificationResult {
	reasons := []string{}
	if env.PayloadType == "" || env.Payload == "" {
		return VerificationResult{Valid: false, Reasons: []string{"missing DSSE payload"}}
	}
	if minSignatures < 1 {
		minSignatures = 1
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return VerificationResult{Valid: false, Reasons: []string{"invalid base64 payload"}}
	}
	verified := 0
	for _, sig := range env.Signatures {
		pub := trustedKeys[sig.KeyID]
		if pub == "" {
			continue
		}
		if verifyDetached(pub, payload, sig.Sig) {
			verified++
		}
	}
	if verified < minSignatures {
		reasons = append(reasons, "insufficient trusted provenance signatures")
	}
	sum := sha256.Sum256(payload)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO provenance_bundles(bundle_hash, envelope_json, verified_signatures, created_at) VALUES(?,?,?,?)`,
		hex.EncodeToString(sum[:]), mustJSON(env), verified, time.Now().UTC().Unix()); err != nil {
		s.log.Warn("store provenance bundle failed", "err", err)
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "provenance thresholds satisfied")
	}
	return VerificationResult{Valid: len(reasons) == 0, Reasons: reasons, Digest: hex.EncodeToString(sum[:])}
}

func (s *Service) IsRevoked(ctx context.Context, id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM revocations WHERE ref_id = ?`, id).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return count > 0, nil
}

func hashManifest(m artifact.Manifest) string {
	copyM := m
	copyM.Signature = ""
	b, _ := json.Marshal(copyM)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func verifyDetached(pubKeyHex string, payload any, sigB64 string) bool {
	pub, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	var b []byte
	switch p := payload.(type) {
	case string:
		b = []byte(p)
	case []byte:
		b = p
	default:
		b = []byte(fmt.Sprint(payload))
	}
	return ed25519.Verify(ed25519.PublicKey(pub), b, sig)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
