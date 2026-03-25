package identity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/blockchain"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
)

type Kind string

const (
	DeviceIdentity   Kind = "device"
	ProviderIdentity Kind = "provider"
	DeveloperID      Kind = "developer"
	OperatorID       Kind = "operator"
)

type Record struct {
	ID         string            `json:"id"`
	Kind       Kind              `json:"kind"`
	PublicKey  string            `json:"public_key"`
	Metadata   map[string]string `json:"metadata"`
	TrustTier  string            `json:"trust_tier"`
	AnchorID   string            `json:"anchor_id"`
	AnchoredTx string            `json:"anchored_tx,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type Service struct {
	db     *storage.DB
	anchor *blockchain.Service
	log    *logging.Logger
}

func NewService(db *storage.DB, anchor *blockchain.Service, log *logging.Logger) *Service {
	return &Service{db: db, anchor: anchor, log: log}
}

func (s *Service) Register(ctx context.Context, kind Kind, pubKey string, metadata map[string]string, requestAnchor bool) (Record, error) {
	if pubKey == "" {
		return Record{}, fmt.Errorf("public_key required")
	}
	now := time.Now().UTC()
	r := Record{Kind: kind, PublicKey: pubKey, Metadata: metadata, CreatedAt: now, UpdatedAt: now}
	r.ID = deriveID(kind, pubKey, metadata)
	if kind == DeviceIdentity {
		if metadata["tpm"] == "true" {
			r.TrustTier = "hardware_rooted"
		} else {
			r.TrustTier = "software_fallback"
		}
	} else {
		r.TrustTier = "signing_identity"
	}
	metaJSON, _ := json.Marshal(metadata)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO identities(id, kind, public_key, metadata_json, trust_tier, created_at, updated_at)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET public_key=excluded.public_key, metadata_json=excluded.metadata_json, trust_tier=excluded.trust_tier, updated_at=excluded.updated_at`,
		r.ID, string(r.Kind), r.PublicKey, string(metaJSON), r.TrustTier, r.CreatedAt.Unix(), r.UpdatedAt.Unix())
	if err != nil {
		return Record{}, fmt.Errorf("store identity: %w", err)
	}
	if requestAnchor {
		a := s.anchor.EncodeAnchor("identity", digestAny(metaJSON), r.ID, string(r.Kind))
		created, err := s.anchor.CreateAnchor(ctx, a)
		if err != nil {
			return Record{}, err
		}
		r.AnchorID = created.ID
		r.AnchoredTx = created.TxID
		_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO anchors(anchor_id, kind, digest, ref, issuer_id, txid, created_at) VALUES(?,?,?,?,?,?,?)`, created.ID, created.Kind, created.Digest, created.Ref, created.IssuerID, created.TxID, created.CreatedAt.Unix())
	}
	s.log.Info("identity registered", "kind", r.Kind, "identity_id", r.ID, "trust_tier", r.TrustTier)
	return r, nil
}

func (s *Service) Get(ctx context.Context, id string) (Record, error) {
	var kind, pub, metaJSON, tier string
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT kind, public_key, metadata_json, trust_tier, created_at, updated_at FROM identities WHERE id = ?`, id).Scan(&kind, &pub, &metaJSON, &tier, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return Record{}, fmt.Errorf("identity not found")
		}
		return Record{}, err
	}
	meta := map[string]string{}
	_ = json.Unmarshal([]byte(metaJSON), &meta)
	return Record{ID: id, Kind: Kind(kind), PublicKey: pub, Metadata: meta, TrustTier: tier, CreatedAt: time.Unix(createdAt, 0).UTC(), UpdatedAt: time.Unix(updatedAt, 0).UTC()}, nil
}

func deriveID(kind Kind, pub string, metadata map[string]string) string {
	metaJSON, _ := json.Marshal(metadata)
	sum := sha256.Sum256([]byte(string(kind) + ":" + pub + ":" + string(metaJSON)))
	return hex.EncodeToString(sum[:16])
}

func digestAny(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
