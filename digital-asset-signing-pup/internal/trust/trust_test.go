package trust

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/artifact"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/logging"
	"github.com/edtubbs/pups/digital-asset-signing-pup/internal/storage"
)

func TestManifestVerification(t *testing.T) {
	db, err := storage.Open(context.Background(), "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	svc := NewService(db, logging.New("error"))

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	m := artifact.Manifest{SchemaVersion: "manifest/v1", ArtifactDigest: "d", ProviderIdentity: "p", Version: "1.0.0"}
	digest := hashManifest(m)
	sig := ed25519.Sign(priv, []byte(digest))
	m.Signature = base64.StdEncoding.EncodeToString(sig)

	out := svc.VerifyManifest(context.Background(), m, hex.EncodeToString(pub))
	if !out.Valid {
		t.Fatalf("expected manifest valid: %v", out.Reasons)
	}
}
