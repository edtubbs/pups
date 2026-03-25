package model

import (
"context"
"os"
"path/filepath"
"testing"

"github.com/edtubbs/pups/exchange-pup/internal/types"
)

func TestFakeModelPredict(t *testing.T) {
f := NewFake()
md := Metadata{ModelVersion: "m1", FeatureSchemaVersion: "v1", Horizon: "1h"}
if err := f.LoadModel("", md); err != nil {
t.Fatal(err)
}
ranked, conf, err := f.Predict(context.Background(), types.FeatureVector{SchemaVersion: "v1", Values: map[string]float64{"ret_5": 0.1}})
if err != nil {
t.Fatal(err)
}
if len(ranked) == 0 || conf <= 0 {
t.Fatal("expected ranked actions")
}
}

func TestSchemaMismatchGolden(t *testing.T) {
if err := ValidateSchema("v1", "v2"); err == nil {
t.Fatal("expected mismatch error")
}
}

func TestLoadMetadata(t *testing.T) {
dir := t.TempDir()
p := filepath.Join(dir, "metadata.json")
if err := os.WriteFile(p, []byte(`{"model_version":"m1","feature_schema_version":"v1","target_labels":["HOLD"]}`), 0o600); err != nil {
t.Fatal(err)
}
md, err := LoadMetadata(p)
if err != nil {
t.Fatal(err)
}
if md.ModelVersion != "m1" {
t.Fatal("bad model version")
}
}
