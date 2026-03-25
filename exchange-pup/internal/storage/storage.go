package storage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"

	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db  *sql.DB
	log *slog.Logger
}

func Open(path string, log *slog.Logger) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, log: log}
	if err := s.Migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate() error {
	b, err := migrations.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if _, err := s.db.Exec(string(b)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	return nil
}

func (s *Store) SaveConfigSnapshot(ctx context.Context, cfg map[string]any) error {
	b, _ := json.Marshal(cfg)
	_, err := s.db.ExecContext(ctx, `INSERT INTO config_snapshots(ts, config_json) VALUES (?, ?)`, time.Now().Unix(), string(b))
	return err
}

func (s *Store) InsertTick(ctx context.Context, t types.MarketTick) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO market_ticks(ts, exchange, symbol, price, bid, ask, volume) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.Timestamp.Unix(), t.Exchange, t.Symbol, t.Price, t.Bid, t.Ask, t.Volume)
	return err
}

func (s *Store) InsertCandle(ctx context.Context, c types.Candle) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO market_candles(ts, exchange, symbol, interval, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CloseTime.Unix(), c.Exchange, c.Symbol, c.Interval, c.Open, c.High, c.Low, c.Close, c.Volume)
	return err
}

func (s *Store) InsertOrderBook(ctx context.Context, b types.OrderBook) error {
	payload, _ := json.Marshal(b)
	_, err := s.db.ExecContext(ctx, `INSERT INTO market_books(ts, exchange, symbol, depth_json) VALUES (?, ?, ?, ?)`,
		b.Timestamp.Unix(), b.Exchange, b.Symbol, string(payload))
	return err
}

func (s *Store) InsertFeatureRow(ctx context.Context, fv types.FeatureVector) error {
	values, _ := json.Marshal(fv.Values)
	contextJSON, _ := json.Marshal(fv.Context)
	_, err := s.db.ExecContext(ctx, `INSERT INTO feature_rows(ts, symbol, feature_schema_version, values_json, context_json) VALUES (?, ?, ?, ?, ?)`,
		fv.Timestamp.Unix(), fv.Symbol, fv.SchemaVersion, string(values), string(contextJSON))
	return err
}

func (s *Store) InsertSignal(ctx context.Context, r types.Recommendation) error {
	ranked, _ := json.Marshal(r.RankedActions)
	reasons, _ := json.Marshal(r.ReasonCodes)
	policyReasons, _ := json.Marshal(r.PolicyDecision.ReasonCodes)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO signals(
id, ts, action, confidence, expected_edge, horizon,
ranked_actions_json, reason_codes_json, reason_summary,
model_version, feature_schema_version, policy_allowed, policy_reasons_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.SignalID, r.Timestamp.Unix(), r.Action, r.Confidence, r.ExpectedEdge, r.Horizon,
		string(ranked), string(reasons), r.ReasonSummary, r.ModelVersion, r.FeatureSchema, boolToInt(r.PolicyDecision.Allowed), string(policyReasons))
	return err
}

func (s *Store) InsertPolicyDecision(ctx context.Context, signalID string, d types.PolicyDecision) error {
	reasons, _ := json.Marshal(d.ReasonCodes)
	_, err := s.db.ExecContext(ctx, `INSERT INTO policy_decisions(ts, signal_id, allowed, mode, requires_approval, dry_run, reason_codes_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), signalID, boolToInt(d.Allowed), d.Mode, boolToInt(d.RequiresApproval), boolToInt(d.DryRun), string(reasons))
	return err
}

func (s *Store) InsertExecution(ctx context.Context, signalID, executor, status string, req, resp any) error {
	reqB, _ := json.Marshal(req)
	respB, _ := json.Marshal(resp)
	_, err := s.db.ExecContext(ctx, `INSERT INTO executions(ts, signal_id, executor, status, request_json, response_json) VALUES (?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), signalID, executor, status, string(reqB), string(respB))
	return err
}

func (s *Store) InsertApproval(ctx context.Context, signalID, approver string, approved bool, notes string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO approvals(ts, signal_id, approver, approved, notes) VALUES (?, ?, ?, ?, ?)`,
		time.Now().Unix(), signalID, approver, boolToInt(approved), notes)
	return err
}

func (s *Store) LatestSignals(ctx context.Context, limit int) ([]types.Recommendation, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, ts, action, confidence, expected_edge, horizon, ranked_actions_json,
       reason_codes_json, reason_summary, model_version, feature_schema_version,
       policy_allowed, policy_reasons_json
FROM signals ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]types.Recommendation, 0, limit)
	for rows.Next() {
		var (
			r             types.Recommendation
			ts            int64
			rankedJSON    string
			reasonsJSON   string
			policyAllowed int
			policyReasons string
		)
		if err := rows.Scan(&r.SignalID, &ts, &r.Action, &r.Confidence, &r.ExpectedEdge, &r.Horizon,
			&rankedJSON, &reasonsJSON, &r.ReasonSummary, &r.ModelVersion, &r.FeatureSchema, &policyAllowed, &policyReasons); err != nil {
			return nil, err
		}
		r.Timestamp = time.Unix(ts, 0).UTC()
		r.PolicyDecision.Allowed = policyAllowed == 1
		_ = json.Unmarshal([]byte(rankedJSON), &r.RankedActions)
		_ = json.Unmarshal([]byte(reasonsJSON), &r.ReasonCodes)
		_ = json.Unmarshal([]byte(policyReasons), &r.PolicyDecision.ReasonCodes)
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
