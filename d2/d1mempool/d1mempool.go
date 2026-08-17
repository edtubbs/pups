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
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// d1mempool feeds UNCONFIRMED D1 transactions to the d2-node, which wraps
// each raw D1 transaction in a D1Relay D2 transaction, gossips it through
// the D2 mempool and includes it in D2 blocks.
//
// The feed is a FILE DROP, exactly like the confirmed-block feed: d2-node
// has no D1 relay JSON-RPC method (d2_sendRawTransaction only accepts
// canonical D2 transaction bytes), so its only D1 ingress is the directory
// passed as --d1-follow-dir. The node's d1follow module sweeps that
// directory once a second, draining *.tx files before <height>.blk files,
// and unlinks each file once it has been handed to the relay:
//
//   - <height>.blk — canonical raw D1 block bytes (written by d1follower)
//   - <txid>.tx    — canonical raw D1 transaction bytes (written here)
//
// Flow, once per poll:
//   - core RPC (core-rpc dependency) getrawmempool -> txids
//   - getrawtransaction <txid> 0 -> canonical raw D1 transaction hex
//   - hex-decode and drop <txid>.tx into the follow dir
//
// Writes are atomic: each transaction is staged in a sibling directory on
// the same filesystem and rename()d into place, so the node never observes
// a partial .tx file. A transaction is dropped once; txids that leave D1's
// mempool (mined or evicted) are forgotten — and their still-unconsumed
// files removed — so neither the tracking set nor the drop dir grows
// without bound. Drops are capped per poll so a large mempool backlog
// cannot flood the node.
//
// A file drop has no accept/reject reply, so the relay outcome is read from
// the node's own Prometheus exposition (--metrics-listen in run.sh) rather
// than counted here. A nonzero rejected counter is expected: the node
// mirrors only relay-tier spends and suppresses duplicates by D1 txid.
//
// The drop directory and file extension can be overridden with
// D1MEMPOOL_DROP_DIR and D1MEMPOOL_DROP_EXT for node builds that expect a
// different location or suffix.

const (
	d1RPCUser = "dogebox_core_pup_temporary_static_username"
	d1RPCPass = "dogebox_core_pup_temporary_static_password"

	// The node's --d1-follow-dir (see run.sh in pup.nix), shared with
	// d1follower's confirmed .blk drops, and a staging dir on the same
	// filesystem for atomic renames.
	defaultDropDir = "/storage/d1follow"
	stagingDir     = "/storage/d1mempool.staging"
	defaultDropExt = ".tx"

	// The d2-node Prometheus exposition (--metrics-listen in run.sh).
	nodeMetricsURL = "http://127.0.0.1:42072/metrics"

	pollInterval = 5 * time.Second
	startupDelay = 15 * time.Second

	// Backpressure: cap the transactions dropped in a single poll. The rest
	// are picked up by later polls (they stay in D1's mempool).
	maxSubmitsPerTick = 200
)

// nodeRelayMetrics maps the node's D1 relay collectors to the Dogebox GUI
// metric names declared in manifest.json.
var nodeRelayMetrics = map[string]string{
	"d2_d1_relay_txs_total":          "d1_relayed_total",
	"d2_d1_relay_rejected_total":     "d1_relay_failed",
	"d2_d1_relay_bytes_total":        "d1_relay_bytes_total",
	"d2_d1_relay_capacity_share_bps": "d1_relay_capacity_share_bps",
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

	dropDir string
	dropExt string

	// relayed holds the txids already dropped into the follow dir, pruned
	// to D1's current mempool on every poll.
	relayed map[string]bool

	// Metrics
	mempoolTxs int64
	pending    int64
	// Relay counters scraped from the node's Prometheus exposition, keyed
	// by the GUI metric name (see nodeRelayMetrics).
	nodeMetrics map[string]float64
}

func newFeeder() *Feeder {
	host := os.Getenv("DBX_IFACE_CORE_RPC_HOST")
	port := os.Getenv("DBX_IFACE_CORE_RPC_PORT")

	dropDir := defaultDropDir
	if dir := strings.TrimSpace(os.Getenv("D1MEMPOOL_DROP_DIR")); dir != "" {
		dropDir = dir
	}
	dropExt := defaultDropExt
	if ext := strings.TrimSpace(os.Getenv("D1MEMPOOL_DROP_EXT")); ext != "" {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		dropExt = ext
	}

	return &Feeder{
		client:      &http.Client{Timeout: 60 * time.Second},
		d1RPCURL:    fmt.Sprintf("http://%s:%s/", host, port),
		dropDir:     dropDir,
		dropExt:     dropExt,
		relayed:     make(map[string]bool),
		nodeMetrics: make(map[string]float64),
	}
}

func (f *Feeder) d1RPCCall(method string, params []interface{}, result interface{}) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "1.0",
		ID:      "d1mempool",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", f.d1RPCURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.SetBasicAuth(d1RPCUser, d1RPCPass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
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

func (f *Feeder) getRawMempool() ([]string, error) {
	var txids []string
	// Verbose false: a plain array of txids.
	err := f.d1RPCCall("getrawmempool", []interface{}{false}, &txids)
	return txids, err
}

// getRawTransaction returns the canonical raw D1 transaction bytes — the
// exact payload the node wraps in a D1Relay D2 transaction. Mempool
// transactions are retrievable this way with or without txindex.
func (f *Feeder) getRawTransaction(txid string) ([]byte, error) {
	var rawHex string
	// Verbosity 0 (false): serialized, hex-encoded transaction data.
	if err := f.d1RPCCall("getrawtransaction", []interface{}{txid, false}, &rawHex); err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(rawHex))
	if err != nil {
		return nil, fmt.Errorf("tx %s: bad raw hex: %w", txid, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("tx %s: empty raw transaction", txid)
	}
	return raw, nil
}

// isTxid rejects anything that is not a 64-character hex string, so a value
// returned by the D1 RPC can never build a path outside the drop dir.
func isTxid(txid string) bool {
	if len(txid) != 64 {
		return false
	}
	_, err := hex.DecodeString(txid)
	return err == nil
}

func (f *Feeder) dropPath(txid string) string {
	return filepath.Join(f.dropDir, txid+f.dropExt)
}

// writeTx atomically drops <txid>.tx into the follow dir: the bytes are
// staged on the same filesystem and rename()d into place, so the node's
// directory sweep never sees a partial file.
func (f *Feeder) writeTx(txid string, raw []byte) error {
	staged := filepath.Join(stagingDir, txid+f.dropExt)
	if err := os.WriteFile(staged, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(staged, f.dropPath(txid)); err != nil {
		os.Remove(staged)
		return err
	}
	return nil
}

// pendingDrops lists the txids of unconsumed .tx files in the drop dir (the
// node unlinks each file once it has handed it to the relay).
func (f *Feeder) pendingDrops() map[string]bool {
	pending := make(map[string]bool)
	entries, err := os.ReadDir(f.dropDir)
	if err != nil {
		log.Printf("Error reading the drop dir %s: %v", f.dropDir, err)
		return pending
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, f.dropExt) {
			continue
		}
		if txid := strings.TrimSuffix(name, f.dropExt); isTxid(txid) {
			pending[txid] = true
		}
	}
	return pending
}

// scrapeNodeMetrics reads the node's D1 relay counters from its Prometheus
// exposition. The file drop has no reply, so these are the only authority
// on how many transactions the relay actually accepted or rejected.
func (f *Feeder) scrapeNodeMetrics() {
	resp, err := f.client.Get(nodeMetricsURL)
	if err != nil {
		return // node not up yet or metrics listener disabled
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "#") {
			continue // HELP/TYPE comments
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// Skip labelled series: the relay collectors are plain counters and
		// gauges, and a labelled sample has no single value to show as one
		// UI metric.
		guiName, ok := nodeRelayMetrics[fields[0]]
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		f.nodeMetrics[guiName] = val
	}
}

func (f *Feeder) tick() {
	txids, err := f.getRawMempool()
	if err != nil {
		log.Printf("Error getting the D1 mempool: %v", err)
		return
	}
	f.mempoolTxs = int64(len(txids))

	inMempool := make(map[string]bool, len(txids))
	for _, txid := range txids {
		inMempool[txid] = true
	}

	// Forget transactions that left D1's mempool (mined or evicted) so the
	// tracking set cannot grow without bound, and withdraw the ones the
	// node has not consumed yet so stale files cannot pile up.
	onDisk := f.pendingDrops()
	for txid := range f.relayed {
		if inMempool[txid] {
			continue
		}
		if onDisk[txid] {
			if err := os.Remove(f.dropPath(txid)); err != nil && !os.IsNotExist(err) {
				log.Printf("Error removing the stale D1 tx file for %s: %v", txid, err)
				continue
			}
			delete(onDisk, txid)
		}
		delete(f.relayed, txid)
	}

	dropped := 0
	for _, txid := range txids {
		if dropped >= maxSubmitsPerTick {
			break
		}
		if f.relayed[txid] || onDisk[txid] {
			continue
		}
		if !isTxid(txid) {
			log.Printf("Skipping the malformed txid %q from the D1 mempool", txid)
			continue
		}
		raw, err := f.getRawTransaction(txid)
		if err != nil {
			// A transaction can be mined or evicted between the mempool
			// listing and the fetch; it is retried on the next poll if it
			// is still pending.
			log.Printf("Error fetching D1 tx %s: %v", txid, err)
			continue
		}
		if err := f.writeTx(txid, raw); err != nil {
			log.Printf("Error dropping D1 tx %s into %s: %v", txid, f.dropDir, err)
			continue
		}
		f.relayed[txid] = true
		onDisk[txid] = true
		dropped++
	}

	f.pending = int64(len(onDisk))
	if dropped > 0 {
		log.Printf("Dropped %d unconfirmed D1 transactions into %s (%d in the D1 mempool, %d awaiting the node's sweep)",
			dropped, f.dropDir, f.mempoolTxs, f.pending)
	}
}

func (f *Feeder) submitMetrics() {
	jsonData := map[string]interface{}{
		"d1_mempool_txs":   map[string]interface{}{"value": f.mempoolTxs},
		"d1_relay_pending": map[string]interface{}{"value": f.pending},
	}
	for name, value := range f.nodeMetrics {
		jsonData[name] = map[string]interface{}{"value": int64(value)}
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

// clearStaging removes half-written files left behind by a crash: a staged
// file was never visible to the node, so it can always be discarded.
func clearStaging() {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(stagingDir, e.Name())
		if err := os.Remove(path); err != nil {
			log.Printf("Error removing the stale staged file %s: %v", path, err)
		}
	}
}

func main() {
	log.Println("Sleeping to give the D1 core and D2 nodes time to start..")
	time.Sleep(startupDelay)

	f := newFeeder()
	for _, dir := range []string{f.dropDir, stagingDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("Cannot create %s: %v", dir, err)
		}
	}
	clearStaging()

	// Files left in the drop dir by a previous run are still valid: the
	// node consumes them on its next sweep.
	for txid := range f.pendingDrops() {
		f.relayed[txid] = true
	}

	log.Printf("Reading the D1 mempool from core RPC at %s", f.d1RPCURL)
	log.Printf("Dropping unconfirmed D1 transactions as <txid>%s into %s", f.dropExt, f.dropDir)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		f.tick()
		f.scrapeNodeMetrics()
		f.submitMetrics()
	}
}
