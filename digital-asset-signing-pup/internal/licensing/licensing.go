package licensing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
)

type Entitlement struct {
	ID           string            `json:"id"`
	ArtifactID   string            `json:"artifact_id"`
	SubjectType  string            `json:"subject_type"`
	SubjectID    string            `json:"subject_id"`
	Status       string            `json:"status"`
	StartsAt     time.Time         `json:"starts_at"`
	EndsAt       time.Time         `json:"ends_at"`
	Metadata     map[string]string `json:"metadata"`
	Transferable bool              `json:"transferable"`
}

type Service struct {
	db               *storage.DB
	transfersEnabled bool
	log              *logging.Logger
}

func NewService(db *storage.DB, transfersEnabled bool, log *logging.Logger) *Service {
	return &Service{db: db, transfersEnabled: transfersEnabled, log: log}
}

func (s *Service) Create(ctx context.Context, ent Entitlement) (Entitlement, error) {
	if ent.ID == "" || ent.SubjectID == "" || ent.ArtifactID == "" {
		return Entitlement{}, fmt.Errorf("entitlement id, subject_id, artifact_id required")
	}
	if ent.StartsAt.IsZero() {
		ent.StartsAt = time.Now().UTC()
	}
	if ent.EndsAt.IsZero() {
		ent.EndsAt = ent.StartsAt.Add(365 * 24 * time.Hour)
	}
	if ent.Status == "" {
		ent.Status = "active"
	}
	meta, _ := json.Marshal(ent.Metadata)
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO entitlements(entitlement_id, artifact_id, subject_type, subject_id, status, starts_at, ends_at, metadata_json, transferable) VALUES(?,?,?,?,?,?,?,?,?)`,
		ent.ID, ent.ArtifactID, ent.SubjectType, ent.SubjectID, ent.Status, ent.StartsAt.Unix(), ent.EndsAt.Unix(), string(meta), boolToInt(ent.Transferable))
	if err != nil {
		return Entitlement{}, err
	}
	s.log.Info("entitlement created", "entitlement_id", ent.ID, "artifact_id", ent.ArtifactID)
	return ent, nil
}

func (s *Service) Transfer(ctx context.Context, id, toSubject string) error {
	if !s.transfersEnabled {
		return fmt.Errorf("transfers disabled by policy")
	}
	ent, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !ent.Transferable {
		return fmt.Errorf("entitlement is not transferable")
	}
	if ent.Status != "active" {
		return fmt.Errorf("entitlement is not active")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE entitlements SET subject_id = ? WHERE entitlement_id = ?`, toSubject, id)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO transfers(entitlement_id, from_subject_id, to_subject_id, timestamp) VALUES(?,?,?,?)`, id, ent.SubjectID, toSubject, time.Now().UTC().Unix())
	return nil
}

func (s *Service) Revoke(ctx context.Context, id, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE entitlements SET status='revoked' WHERE entitlement_id = ?`, id)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO revocations(ref_id, kind, reason, timestamp) VALUES(?,?,?,?)`, id, "entitlement", reason, time.Now().UTC().Unix())
	return nil
}

func (s *Service) Get(ctx context.Context, id string) (Entitlement, error) {
	var e Entitlement
	var startsAt, endsAt int64
	var metadataJSON string
	var transferable int
	err := s.db.QueryRowContext(ctx, `SELECT artifact_id, subject_type, subject_id, status, starts_at, ends_at, metadata_json, transferable FROM entitlements WHERE entitlement_id = ?`, id).
		Scan(&e.ArtifactID, &e.SubjectType, &e.SubjectID, &e.Status, &startsAt, &endsAt, &metadataJSON, &transferable)
	if err != nil {
		if err == sql.ErrNoRows {
			return Entitlement{}, fmt.Errorf("entitlement not found")
		}
		return Entitlement{}, err
	}
	e.ID = id
	e.StartsAt = time.Unix(startsAt, 0).UTC()
	e.EndsAt = time.Unix(endsAt, 0).UTC()
	e.Transferable = transferable == 1
	e.Metadata = map[string]string{}
	_ = json.Unmarshal([]byte(metadataJSON), &e.Metadata)
	return e, nil
}

func (s *Service) Evaluate(ctx context.Context, id, subject string, now time.Time) (bool, string, error) {
	e, err := s.Get(ctx, id)
	if err != nil {
		return false, "not_found", err
	}
	if e.Status != "active" {
		return false, "inactive", nil
	}
	if now.Before(e.StartsAt) || now.After(e.EndsAt) {
		return false, "expired", nil
	}
	if e.SubjectID != subject {
		return false, "subject_mismatch", nil
	}
	return true, "ok", nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
