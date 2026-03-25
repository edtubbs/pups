package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Manifest struct {
	SchemaVersion         string            `json:"schema_version"`
	ArtifactDigest        string            `json:"artifact_digest"`
	Version               string            `json:"version"`
	ContentType           string            `json:"content_type"`
	ProviderIdentity      string            `json:"provider_identity"`
	TargetDeviceClass     string            `json:"target_device_class"`
	TargetUserClass       string            `json:"target_user_class"`
	CustomizationMetadata map[string]string `json:"customization_metadata"`
	LicenseID             string            `json:"license_id"`
	Dependencies          []string          `json:"dependencies"`
	SupersessionRefs      []string          `json:"supersession_refs"`
	RevocationRefs        []string          `json:"revocation_refs"`
	PriceRef              string            `json:"price_ref"`
	ProvenanceRefs        []string          `json:"provenance_refs"`
	Signature             string            `json:"signature"`
}

func DigestFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func EnsureStaging(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "/" {
		return fmt.Errorf("refusing unsafe staging path")
	}
	return os.MkdirAll(dir, 0o750)
}
