package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/config"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
)

func TestFetchAndVerify(t *testing.T) {
	content := []byte("artifact-bytes")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := NewService(config.DistributionConfig{CacheDir: dir, BackendURLs: []string{srv.URL}, TimeoutSeconds: 2}, logging.New("error"))
	res, err := s.FetchByDigest(context.Background(), digest, "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res.Path != filepath.Join(dir, digest) {
		t.Fatalf("unexpected path %s", res.Path)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatalf("missing fetched artifact: %v", err)
	}
}
