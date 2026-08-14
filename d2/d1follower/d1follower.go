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
	"sort"
	"strconv"
	"strings"
	"time"
)

// d1follower feeds the d2-node's d1follow module. d2-node does NOT ingest
// D1 data over RPC: it is started with --d1-follow-dir <dir> and polls that
// directory for files named <height>.blk containing CANONICAL RAW D1 BLOCK
// BYTES (exactly what Core's `getblock <hash> 0` returns). The node ingests
// each block, derives the migration index (Tier-B candidates, spent
// markers, resume checkpoint), and DELETES the file once it is durably
// applied.
//
// This service runs inside the d2 pup (the drop directory must be on the
// node's own /storage; pups do not share storage), polls the core pup's
// RPC (core-rpc dependency) for new confirmed D1 blocks, and writes them
// into the follow dir. Writes are atomic: the block is staged in a sibling
// directory on the same filesystem and rename()d into place, so the
// follower never observes a partial .blk file.
//
// Progress tracking:
//   - The snapshot metadata (/storage/utxo.dat.meta.json, written by
//     run.sh) pins the epoch: blocks are fed starting at base_height+1.
//   - A cursor file records the epoch base hash and the next height to
//     write. On an epoch change (fresh snapshot, new base hash) the cursor
//     resets and stale pending .blk files are discarded.
//   - The node's own follower checkpoint (the d2_d1_height gauge on its
//     /metrics listener) is the source of truth: the cursor fast-forwards
//     to it after restarts, so blocks the node already applied are never
//     re-fed.
//
// Only blocks buried by `confirmations` are fed, so a shallow D1 reorg
// cannot push an orphaned block into the migration index.

const (
	d1RPCUser = "dogebox_core_pup_temporary_static_username"
	d1RPCPass = "dogebox_core_pup_temporary_static_password"

	followDir    = "/storage/d1follow"
	stagingDir   = "/storage/d1follow.staging"
	cursorPath   = "/storage/d1follower.cursor"
	snapshotMeta = "/storage/utxo.dat.meta.json"

	// The d2-node Prometheus exposition (--metrics-listen in run.sh).
	nodeMetricsURL = "http://127.0.0.1:42072/metrics"

	pollInterval  = 10 * time.Second
	confirmations = 12 // reorg safety margin before a block is fed
	maxPending    = 25 // backpressure: max unconsumed .blk files
	startupDelay  = 10 * time.Second
)

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

type snapshotMetadata struct {
	BaseHash   string `json:"base_hash"`
	BaseHeight int64  `json:"base_height"`
}

type cursor struct {
	BaseHash   string `json:"base_hash"`
	NextHeight int64  `json:"next_height"`
}

type Follower struct {
	client   *http.Client
	d1RPCURL string

	cur cursor

	// Metrics
	d1Tip         int64
	blocksWritten int64
	pending       int64
	// All d2_d1_* collectors scraped from the node's Prometheus exposition
	// (§9.9 follower metrics), keyed by metric name. Every entry is
	// forwarded to the Dogebox UI as d1_<suffix>, so new node-side
	// follower collectors show up without code changes here (they still
	// need a matching entry in manifest.json to be displayed).
	nodeMetrics map[string]float64
}

// nodeMetricPrefix selects the node's d1follow collectors.
const nodeMetricPrefix = "d2_d1_"

// nodeD1Height returns the node's ingest checkpoint (d2_d1_height).
func (f *Follower) nodeD1Height() float64 {
	return f.nodeMetrics["d2_d1_height"]
}

func newFollower() *Follower {
	host := os.Getenv("DBX_IFACE_CORE_RPC_HOST")
	port := os.Getenv("DBX_IFACE_CORE_RPC_PORT")
	return &Follower{
		client:      &http.Client{Timeout: 60 * time.Second},
		d1RPCURL:    fmt.Sprintf("http://%s:%s/", host, port),
		nodeMetrics: make(map[string]float64),
	}
}

func (f *Follower) d1RPCCall(method string, params []interface{}, result interface{}) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "1.0",
		ID:      "d1follower",
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
		return fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return json.Unmarshal(rpcResp.Result, result)
}

func (f *Follower) getBlockCount() (int64, error) {
	var count int64
	err := f.d1RPCCall("getblockcount", nil, &count)
	return count, err
}

func (f *Follower) getBlockHash(height int64) (string, error) {
	var hash string
	err := f.d1RPCCall("getblockhash", []interface{}{height}, &hash)
	return hash, err
}

// getRawBlock returns the canonical wire-format block bytes at a height —
// the exact payload the d1follow module expects in a .blk file.
func (f *Follower) getRawBlock(height int64) ([]byte, error) {
	hash, err := f.getBlockHash(height)
	if err != nil {
		return nil, err
	}
	var rawHex string
	// Verbosity 0 (false): serialized, hex-encoded block data.
	if err := f.d1RPCCall("getblock", []interface{}{hash, false}, &rawHex); err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(rawHex))
	if err != nil {
		return nil, fmt.Errorf("block %d: bad raw hex: %w", height, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("block %d: empty raw block", height)
	}
	return raw, nil
}

func readSnapshotMeta() (snapshotMetadata, bool) {
	var meta snapshotMetadata
	data, err := os.ReadFile(snapshotMeta)
	if err != nil {
		return meta, false
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, false
	}
	return meta, meta.BaseHash != ""
}

func loadCursor() (cursor, bool) {
	var c cursor
	data, err := os.ReadFile(cursorPath)
	if err != nil {
		return c, false
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, false
	}
	return c, c.NextHeight > 0
}

func (f *Follower) saveCursor() {
	data, err := json.Marshal(f.cur)
	if err != nil {
		log.Printf("Error marshalling cursor: %v", err)
		return
	}
	tmp := cursorPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("Error writing cursor: %v", err)
		return
	}
	if err := os.Rename(tmp, cursorPath); err != nil {
		log.Printf("Error renaming cursor: %v", err)
	}
}

// pendingBlocks lists the heights of unconsumed .blk files in the follow
// dir (the node deletes each file once it is durably applied).
func pendingBlocks() []int64 {
	entries, err := os.ReadDir(followDir)
	if err != nil {
		return nil
	}
	var heights []int64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".blk") {
			continue
		}
		h, err := strconv.ParseInt(strings.TrimSuffix(name, ".blk"), 10, 64)
		if err != nil {
			continue
		}
		heights = append(heights, h)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	return heights
}

func clearPendingBlocks() {
	for _, h := range pendingBlocks() {
		path := filepath.Join(followDir, fmt.Sprintf("%d.blk", h))
		if err := os.Remove(path); err != nil {
			log.Printf("Error removing stale block file %s: %v", path, err)
		}
	}
}

// writeBlock atomically drops <height>.blk into the follow dir: the bytes
// are staged on the same filesystem and rename()d into place so the node's
// directory poll never sees a partial file.
func writeBlock(height int64, raw []byte) error {
	staged := filepath.Join(stagingDir, fmt.Sprintf("%d.blk", height))
	if err := os.WriteFile(staged, raw, 0o644); err != nil {
		return err
	}
	final := filepath.Join(followDir, fmt.Sprintf("%d.blk", height))
	if err := os.Rename(staged, final); err != nil {
		os.Remove(staged)
		return err
	}
	return nil
}

// syncEpoch aligns the cursor with the current snapshot epoch. On an epoch
// change (new base hash after the fortnightly reset) the cursor restarts at
// base_height+1 and stale pending files from the old epoch are discarded.
// Without snapshot metadata yet, the follower waits: feeding blocks with no
// pinned base would hand the node an unanchored history.
func (f *Follower) syncEpoch() bool {
	meta, ok := readSnapshotMeta()
	if !ok {
		return false
	}
	if f.cur.BaseHash == meta.BaseHash && f.cur.NextHeight > 0 {
		return true
	}
	if c, ok := loadCursor(); ok && c.BaseHash == meta.BaseHash {
		f.cur = c
		return true
	}
	if f.cur.BaseHash != "" {
		log.Printf("Snapshot epoch changed (base %s -> %s); resetting cursor", f.cur.BaseHash, meta.BaseHash)
		clearPendingBlocks()
	}
	f.cur = cursor{BaseHash: meta.BaseHash, NextHeight: meta.BaseHeight + 1}
	f.saveCursor()
	log.Printf("Following D1 from height %d (snapshot base %s at height %d)",
		f.cur.NextHeight, meta.BaseHash, meta.BaseHeight)
	return true
}

// scrapeNodeMetrics reads the d2-node §9.9 follower collectors from its
// Prometheus exposition: every d2_d1_*-prefixed sample (ingest checkpoint,
// block count, migration candidates, spent markers, and any collectors
// added by newer node versions) is captured for forwarding to the Dogebox
// UI. The checkpoint (d2_d1_height) also fast-forwards the writer cursor
// so already-applied blocks are never re-fed.
func (f *Follower) scrapeNodeMetrics() {
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
		name := fields[0]
		// Skip labelled series: the follower collectors are plain
		// gauges/counters, and a labelled sample has no single value to
		// show as one UI metric.
		if !strings.HasPrefix(name, nodeMetricPrefix) || strings.ContainsAny(name, "{}") {
			continue
		}
		val, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		f.nodeMetrics[name] = val
	}

	// The node's checkpoint is authoritative: never re-feed applied blocks.
	if ckpt := int64(f.nodeD1Height()); ckpt > 0 && ckpt+1 > f.cur.NextHeight {
		f.cur.NextHeight = ckpt + 1
		f.saveCursor()
	}
}

func (f *Follower) tick() {
	f.scrapeNodeMetrics()

	if !f.syncEpoch() {
		log.Println("No snapshot metadata yet; waiting for the epoch base before following D1")
		return
	}

	count, err := f.getBlockCount()
	if err != nil {
		log.Printf("Error getting D1 block count: %v", err)
		return
	}
	f.d1Tip = count

	pending := pendingBlocks()
	f.pending = int64(len(pending))

	// Backpressure: the node deletes each .blk once durably applied, so a
	// growing backlog means it is still catching up — don't pile on.
	budget := maxPending - len(pending)
	safeTip := count - confirmations

	for budget > 0 && f.cur.NextHeight <= safeTip {
		raw, err := f.getRawBlock(f.cur.NextHeight)
		if err != nil {
			log.Printf("Error fetching D1 block %d: %v", f.cur.NextHeight, err)
			return
		}
		if err := writeBlock(f.cur.NextHeight, raw); err != nil {
			log.Printf("Error writing block file for height %d: %v", f.cur.NextHeight, err)
			return
		}
		log.Printf("Dropped D1 block %d (%d bytes) into %s", f.cur.NextHeight, len(raw), followDir)
		f.blocksWritten++
		f.cur.NextHeight++
		f.saveCursor()
		budget--
	}
	f.pending = int64(len(pendingBlocks()))
}

func (f *Follower) submitMetrics() {
	followLag := f.d1Tip - confirmations - int64(f.nodeD1Height())
	if followLag < 0 {
		followLag = 0
	}

	// Metrics computed by this service.
	jsonData := map[string]interface{}{
		"d1_tip":            map[string]interface{}{"value": f.d1Tip},
		"d1_blocks_written": map[string]interface{}{"value": f.blocksWritten},
		"d1_pending":        map[string]interface{}{"value": f.pending},
		"d1_follow_lag":     map[string]interface{}{"value": followLag},
	}

	// Node-side follower collectors, always submitted (as 0 before the
	// first successful scrape) so the UI never shows an empty gauge.
	for _, name := range []string{
		"d2_d1_height",
		"d2_d1_blocks_total",
		"d2_d1_migration_candidates",
		"d2_d1_utxo_spent_total",
	} {
		if _, ok := f.nodeMetrics[name]; !ok {
			f.nodeMetrics[name] = 0
		}
	}

	// Forward every scraped d2_d1_* collector as d1_* (d2_d1_height maps
	// to d1_height, and so on). New node collectors flow through here
	// automatically; add them to manifest.json to display them.
	for name, val := range f.nodeMetrics {
		uiName := "d1_" + strings.TrimPrefix(name, nodeMetricPrefix)
		jsonData[uiName] = map[string]interface{}{"value": int64(val)}
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

	for _, dir := range []string{followDir, stagingDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("Cannot create %s: %v", dir, err)
		}
	}

	f := newFollower()
	log.Printf("Following D1 blocks from core RPC at %s", f.d1RPCURL)
	log.Printf("Dropping raw D1 blocks into %s for the d2-node d1follow module", followDir)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		f.tick()
		f.submitMetrics()
	}
}
