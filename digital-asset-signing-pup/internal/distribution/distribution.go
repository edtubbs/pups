package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
)

type Service struct {
	cfg  config.DistributionConfig
	log  *logging.Logger
	http *http.Client
}

type FetchResult struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	Cached bool   `json:"cached"`
}

func NewService(cfg config.DistributionConfig, log *logging.Logger) *Service {
	return &Service{cfg: cfg, log: log, http: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}}
}

func (s *Service) FetchByDigest(ctx context.Context, digest, ref string) (FetchResult, error) {
	if digest == "" {
		return FetchResult{}, fmt.Errorf("digest required")
	}
	cachePath := filepath.Join(s.cfg.CacheDir, digest)
	if _, err := os.Stat(cachePath); err == nil {
		return FetchResult{Path: cachePath, Digest: digest, Cached: true}, nil
	}
	if err := os.MkdirAll(s.cfg.CacheDir, 0o750); err != nil {
		return FetchResult{}, err
	}
	candidates := []string{}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		candidates = append(candidates, ref)
	}
	for _, base := range s.cfg.BackendURLs {
		candidates = append(candidates, strings.TrimSuffix(base, "/")+"/"+digest)
	}
	if len(candidates) == 0 {
		return FetchResult{}, fmt.Errorf("no distribution backends configured")
	}
	var lastErr error
	for _, u := range candidates {
		fr, err := s.fetchURL(ctx, u, cachePath, digest)
		if err == nil {
			return fr, nil
		}
		lastErr = err
		s.log.Warn("fetch attempt failed", "url", u, "err", err)
	}
	return FetchResult{}, lastErr
}

func (s *Service) fetchURL(ctx context.Context, url, dst, expectedDigest string) (FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FetchResult{}, err
	}
	res, err := s.http.Do(req)
	if err != nil {
		return FetchResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("status %d", res.StatusCode)
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return FetchResult{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), res.Body)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return FetchResult{}, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedDigest {
		_ = os.Remove(tmp)
		return FetchResult{}, fmt.Errorf("digest mismatch")
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return FetchResult{}, err
	}
	return FetchResult{Path: dst, Digest: got, Size: n}, nil
}
