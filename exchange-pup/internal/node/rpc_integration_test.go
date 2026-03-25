package node

import (
"context"
"encoding/json"
"net/http"
"net/http/httptest"
"testing"
"time"

"github.com/edtubbs/pups/exchange-pup/internal/config"
)

func TestSnapshotWithMockRPC(t *testing.T) {
ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
var req map[string]any
_ = json.NewDecoder(r.Body).Decode(&req)
method := req["method"].(string)
var result any
switch method {
case "getblockchaininfo":
result = map[string]any{"chain": "main", "blocks": 10, "headers": 10}
case "getmempoolinfo":
result = map[string]any{"bytes": 100, "size": 2}
case "getbalance":
result = 123.45
case "listtransactions":
result = []any{map[string]any{"txid": "a"}}
default:
result = map[string]any{}
}
_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": nil})
}))
defer ts.Close()

c := NewClient(config.NodeRPCConfig{URL: ts.URL, Timeout: time.Second})
snap, err := c.Snapshot(context.Background())
if err != nil {
t.Fatal(err)
}
if snap.Blocks != 10 || snap.WalletBalance <= 0 {
t.Fatalf("unexpected snapshot: %+v", snap)
}
}
