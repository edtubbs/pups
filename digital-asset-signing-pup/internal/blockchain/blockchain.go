package blockchain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
)

type Client struct {
	cfg  config.BlockchainConfig
	http *http.Client
}

type AnchorRecord struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Digest     string    `json:"digest"`
	Ref        string    `json:"ref"`
	IssuerID   string    `json:"issuer_id"`
	TxID       string    `json:"txid"`
	CreatedAt  time.Time `json:"created_at"`
	Supersedes string    `json:"supersedes,omitempty"`
	Revokes    string    `json:"revokes,omitempty"`
}

type Service struct {
	client       *Client
	allowAnchors bool
	cache        map[string]AnchorRecord
}

func NewClient(cfg config.BlockchainConfig) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.RPCTimeout}}
}

func NewService(c *Client, allowAnchors bool) *Service {
	return &Service{client: c, allowAnchors: allowAnchors, cache: map[string]AnchorRecord{}}
}

func (s *Service) EncodeAnchor(kind, digest, ref, issuer string) AnchorRecord {
	sum := sha256.Sum256([]byte(kind + ":" + digest + ":" + ref + ":" + issuer))
	id := hex.EncodeToString(sum[:16])
	return AnchorRecord{ID: id, Kind: kind, Digest: digest, Ref: ref, IssuerID: issuer, CreatedAt: time.Now().UTC()}
}

func (s *Service) CreateAnchor(ctx context.Context, rec AnchorRecord) (AnchorRecord, error) {
	if !s.allowAnchors {
		return AnchorRecord{}, errors.New("live anchoring disabled by policy")
	}
	txid, err := s.client.anchor(ctx, rec)
	if err != nil {
		return AnchorRecord{}, err
	}
	rec.TxID = txid
	s.cache[rec.ID] = rec
	return rec, nil
}

func (s *Service) VerifyAnchor(_ context.Context, rec AnchorRecord) (bool, string) {
	if rec.ID == "" || rec.Digest == "" || rec.Kind == "" {
		return false, "missing critical anchor fields"
	}
	cached, ok := s.cache[rec.ID]
	if !ok {
		return true, "anchor format valid (not locally cached)"
	}
	if cached.Digest != rec.Digest || cached.Kind != rec.Kind {
		return false, "cached anchor mismatch"
	}
	return true, "anchor verified"
}

func (s *Service) GetCachedAnchor(id string) (AnchorRecord, bool) {
	rec, ok := s.cache[id]
	return rec, ok
}

func (c *Client) anchor(ctx context.Context, rec AnchorRecord) (string, error) {
	if c.cfg.MockMode {
		sum := sha256.Sum256([]byte(rec.ID + rec.Digest + rec.Kind))
		return hex.EncodeToString(sum[:16]), nil
	}
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "1.0",
		"id":      "dasp",
		"method":  "sendtoaddress",
		"params":  []any{"D8H8w8dummyAddressNotForMainnetUse", 0.001, "", "", false, false, 1, "unset", false, c.cfg.OPReturnPrefix + ":" + rec.ID + ":" + rec.Digest},
	})
	if err != nil {
		return "", fmt.Errorf("marshal rpc payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.RPCURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("new rpc request: %w", err)
	}
	req.SetBasicAuth(c.cfg.RPCUser, c.cfg.RPCPass)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("rpc request failed: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("rpc status %d: %s", res.StatusCode, string(body))
	}
	var out struct {
		Result string         `json:"result"`
		Error  map[string]any `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse rpc response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("rpc error: %v", out.Error)
	}
	if out.Result == "" {
		return "", errors.New("empty txid from rpc")
	}
	return out.Result, nil
}
