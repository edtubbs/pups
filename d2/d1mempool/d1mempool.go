package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// d1mempool feeds UNCONFIRMED D1 transactions to the d2-node's D1
// transaction relay. The node wraps each raw D1 transaction in a D1Relay D2
// transaction, gossips it through the D2 mempool and includes it in D2
// blocks; this service is the D1 side of that pipe.
//
// It complements the d1follower service, which feeds CONFIRMED D1 blocks as
// <height>.blk files into the node's --d1-follow-dir. Blocks travel by file
// drop because they are large and ingested in order; unconfirmed
// transactions travel over the node's JSON-RPC write tier instead, so they
// reach the D2 mempool as soon as they appear in D1's.
//
// Flow, once per poll:
//   - core RPC (core-rpc dependency) getrawmempool -> txids
//   - getrawtransaction <txid> 0 -> canonical raw D1 transaction hex
//   - d2-node relay RPC <hex> -> D1Relay D2 transaction
//
// A transaction is submitted once; txids that leave D1's mempool (mined or
// evicted) are forgotten so the tracking set stays bounded. Submissions are
// rate-limited per tick so a large mempool backlog cannot flood the node.
//
// The relay RPC method name is auto-detected from a small candidate list on
// first use (a "method not found" response moves on to the next candidate)
// and can be pinned with D1MEMPOOL_RPC_METHOD. Once every candidate has
// been rejected the service only reports metrics, so an older node build
// without the relay is never hammered with doomed calls and the confirmed
// block path is unaffected.

const (
	d1RPCUser = "dogebox_core_pup_temporary_static_username"
	d1RPCPass = "dogebox_core_pup_temporary_static_password"

	d2RPCURL     = "http://127.0.0.1:42070/"
	rpcTokenPath = "/storage/rpc.token"

	pollInterval = 5 * time.Second
	startupDelay = 15 * time.Second

	// Backpressure: cap the transactions submitted in a single poll. The
	// rest are picked up by later polls (they stay in D1's mempool).
	maxSubmitsPerTick = 200

	// JSON-RPC 2.0 reserved code for an unknown method.
	rpcMethodNotFound = -32601
)

// relayMethodCandidates are tried in order until one is not rejected as an
// unknown method. Set D1MEMPOOL_RPC_METHOD to pin a specific method.
var relayMethodCandidates = []string{
	"d2_sendRawD1Transaction",
	"d2_relayD1Transaction",
	"d2_sendRawD1Tx",
	"d2_submitD1Transaction",
}

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

type Feeder struct {
	client   *http.Client
	d1RPCURL string

	// relayMethod is the detected (or pinned) d2-node relay RPC method,
	// empty until detection succeeds.
	relayMethod string
	// relayUnsupported is set once every candidate has been rejected as an
	// unknown method: the node build has no D1 relay RPC.
	relayUnsupported bool

	// relayed holds the txids already submitted, pruned to D1's current
	// mempool on every poll.
	relayed map[string]bool

	// Metrics
	mempoolTxs   int64
	relayedTotal int64
	failedTotal  int64
	pending      int64
}

func newFeeder() *Feeder {
	host := os.Getenv("DBX_IFACE_CORE_RPC_HOST")
	port := os.Getenv("DBX_IFACE_CORE_RPC_PORT")
	f := &Feeder{
		client:   &http.Client{Timeout: 60 * time.Second},
		d1RPCURL: fmt.Sprintf("http://%s:%s/", host, port),
		relayed:  make(map[string]bool),
	}
	if pinned := strings.TrimSpace(os.Getenv("D1MEMPOOL_RPC_METHOD")); pinned != "" {
		f.relayMethod = pinned
	}
	return f
}

func (f *Feeder) rpcCall(url string, req rpcRequest, auth func(*http.Request), result interface{}) error {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if auth != nil {
		auth(httpReq)
	}

	resp, err := f.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return fmt.Errorf("error unmarshalling RPC response (status %d): %v", resp.StatusCode, err)
	}
	if rpcResp.Error != nil {
		return rpcResp.Error
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(rpcResp.Result, result)
}

func (f *Feeder) d1RPCCall(method string, params []interface{}, result interface{}) error {
	return f.rpcCall(f.d1RPCURL, rpcRequest{
		JSONRPC: "1.0",
		ID:      "d1mempool",
		Method:  method,
		Params:  params,
	}, func(req *http.Request) {
		req.SetBasicAuth(d1RPCUser, d1RPCPass)
	}, result)
}

// d2RPCCall talks to the node's write tier, which requires the bearer token
// the node writes to /storage/rpc.token on first start.
func (f *Feeder) d2RPCCall(method string, params []interface{}, result interface{}) error {
	return f.rpcCall(d2RPCURL, rpcRequest{
		JSONRPC: "2.0",
		ID:      "d1mempool",
		Method:  method,
		Params:  params,
	}, func(req *http.Request) {
		if token := readBearerToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}, result)
}

func readBearerToken() string {
	data, err := os.ReadFile(rpcTokenPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (f *Feeder) getRawMempool() ([]string, error) {
	var txids []string
	// Verbose false: a plain array of txids.
	err := f.d1RPCCall("getrawmempool", []interface{}{false}, &txids)
	return txids, err
}

// getRawTransaction returns the canonical raw D1 transaction hex — exactly
// the bytes the relay wraps in a D1Relay D2 transaction. Mempool
// transactions are retrievable this way with or without txindex.
func (f *Feeder) getRawTransaction(txid string) (string, error) {
	var rawHex string
	// Verbosity 0 (false): serialized, hex-encoded transaction data.
	if err := f.d1RPCCall("getrawtransaction", []interface{}{txid, false}, &rawHex); err != nil {
		return "", err
	}
	rawHex = strings.TrimSpace(rawHex)
	if rawHex == "" {
		return "", fmt.Errorf("tx %s: empty raw transaction", txid)
	}
	if _, err := hex.DecodeString(rawHex); err != nil {
		return "", fmt.Errorf("tx %s: bad raw hex: %w", txid, err)
	}
	return rawHex, nil
}

func isMethodNotFound(err error) bool {
	rerr, ok := err.(*rpcError)
	if !ok {
		return false
	}
	if rerr.Code == rpcMethodNotFound {
		return true
	}
	msg := strings.ToLower(rerr.Message)
	return strings.Contains(msg, "method not found") || strings.Contains(msg, "unknown method")
}

// relay submits one raw D1 transaction to the node, detecting the relay RPC
// method on first use. It reports whether the transaction was accepted.
func (f *Feeder) relay(txid, rawHex string) bool {
	if f.relayUnsupported {
		return false
	}

	candidates := relayMethodCandidates
	if f.relayMethod != "" {
		candidates = []string{f.relayMethod}
	}

	for _, method := range candidates {
		err := f.d2RPCCall(method, []interface{}{rawHex}, nil)
		if err == nil {
			if f.relayMethod != method {
				log.Printf("Using d2-node relay RPC method %s", method)
				f.relayMethod = method
			}
			return true
		}
		if !isMethodNotFound(err) {
			log.Printf("Error relaying D1 tx %s via %s: %v", txid, method, err)
			return false
		}
		if f.relayMethod != "" {
			// A pinned (or previously detected) method that the node does
			// not know: degrade to metrics-only rather than logging the
			// same failure for every mempool transaction, forever.
			f.relayUnsupported = true
			log.Printf("The d2-node build does not expose the relay RPC method %s; "+
				"set D1MEMPOOL_RPC_METHOD to the correct method to enable the mempool feed",
				method)
			return false
		}
		// Detection in progress: try the next candidate name.
	}

	f.relayUnsupported = true
	log.Printf("The d2-node build exposes no D1 transaction relay RPC (tried %s); "+
		"set D1MEMPOOL_RPC_METHOD to the correct method to enable the mempool feed",
		strings.Join(relayMethodCandidates, ", "))
	return false
}

func (f *Feeder) tick() {
	txids, err := f.getRawMempool()
	if err != nil {
		log.Printf("Error getting the D1 mempool: %v", err)
		return
	}
	f.mempoolTxs = int64(len(txids))

	// Forget transactions that left D1's mempool (mined or evicted) so the
	// tracking set cannot grow without bound.
	current := make(map[string]bool, len(txids))
	for _, txid := range txids {
		current[txid] = true
	}
	for txid := range f.relayed {
		if !current[txid] {
			delete(f.relayed, txid)
		}
	}

	submitted := 0
	relayedNow := 0
	for _, txid := range txids {
		if f.relayed[txid] {
			continue
		}
		if f.relayUnsupported || submitted >= maxSubmitsPerTick {
			break
		}
		rawHex, err := f.getRawTransaction(txid)
		if err != nil {
			// A transaction can be mined or evicted between the mempool
			// listing and the fetch; it is retried on the next poll if it
			// is still pending.
			log.Printf("Error fetching D1 tx %s: %v", txid, err)
			continue
		}
		submitted++
		if !f.relay(txid, rawHex) {
			f.failedTotal++
			continue
		}
		f.relayed[txid] = true
		f.relayedTotal++
		relayedNow++
	}

	f.pending = int64(len(txids) - len(f.relayed))
	if f.pending < 0 {
		f.pending = 0
	}
	if relayedNow > 0 {
		log.Printf("Relayed %d unconfirmed D1 transactions to the D2 mempool (%d in the D1 mempool, %d pending)",
			relayedNow, f.mempoolTxs, f.pending)
	}
}

func (f *Feeder) submitMetrics() {
	jsonData := map[string]interface{}{
		"d1_mempool_txs":   map[string]interface{}{"value": f.mempoolTxs},
		"d1_relayed_total": map[string]interface{}{"value": f.relayedTotal},
		"d1_relay_failed":  map[string]interface{}{"value": f.failedTotal},
		"d1_relay_pending": map[string]interface{}{"value": f.pending},
	}

	marshalledData, err := json.Marshal(jsonData)
	if err != nil {
		log.Printf("Error marshalling metrics: %v", err)
		return
	}

	url := fmt.Sprintf("http://%s:%s/dbx/metrics", os.Getenv("DBX_HOST"), os.Getenv("DBX_PORT"))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(marshalledData))
	if err != nil {
		log.Printf("Error creating metrics request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		log.Printf("Error sending metrics: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Unexpected status code when submitting metrics: %d, body: %s", resp.StatusCode, string(body))
	}
}

func main() {
	log.Println("Sleeping to give the D1 core and D2 nodes time to start..")
	time.Sleep(startupDelay)

	f := newFeeder()
	log.Printf("Reading the D1 mempool from core RPC at %s", f.d1RPCURL)
	if f.relayMethod != "" {
		log.Printf("Relaying unconfirmed D1 transactions to %s via %s", d2RPCURL, f.relayMethod)
	} else {
		log.Printf("Relaying unconfirmed D1 transactions to %s (auto-detecting the relay RPC method)", d2RPCURL)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		f.tick()
		f.submitMetrics()
	}
}
