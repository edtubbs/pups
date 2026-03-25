package trust

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/artifact"
)

func TestGoldenManifestFixtureParse(t *testing.T) {
	path := filepath.Join("..", "..", "contrib", "example-manifests", "manifest-v1.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var m artifact.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if m.SchemaVersion == "" || m.ArtifactDigest == "" {
		t.Fatalf("invalid fixture")
	}
}
