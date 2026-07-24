package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The snapshot service exports the Dogecoin (D1) chainstate for the D2
// testnet handoff. On the fortnightly reset schedule (or on demand via
// POST /snapshot/refresh) it:
//
//  1. Records getbestblockhash *before* dumping and verifies the dump's
//     base_hash matches it — d2 performs no base-blockhash, network-magic,
//     or hash_serialized_2 validation, so chain correctness must be
//     verified here (D2 SPEC §11.2 step 1).
//  2. Calls the dumptxoutset RPC (backported to the core build) to write
//     the raw v1 snapshot under /storage.
//  3. Walks the snapshot structurally and ensures the file ends exactly at
//     the last coin record, stripping any trailing bytes — d2's parser
//     fails with a Trailing error otherwise.
//  4. Serves the file plus a metadata document (base blockhash, height,
//     coins count, file sha256) over the core-snapshot HTTP interface.

var storageDirectory = "/storage"

const (
	listenPort      = "28555"
	refreshInterval = 14 * 24 * time.Hour // fortnightly testnet reset

	snapshotDirName  = "d2-snapshot"
	snapshotFileName = "utxo.dat"
	metadataFileName = "metadata.json"

	snapshotVersion = 1
)

var snapshotMagic = []byte{'u', 't', 'x', 'o', 0xff}

// These match the temporary static credentials written by the core pup.
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

type dumpResult struct {
	CoinsWritten int64  `json:"coins_written"`
	BaseHash     string `json:"base_hash"`
	BaseHeight   int64  `json:"base_height"`
	Path         string `json:"path"`
	TxoutsetHash string `json:"txoutset_hash"`
	NChainTx     int64  `json:"nchaintx"`
}

// Metadata is published alongside utxo.dat so the D2 pup can verify the
// download and detect new snapshot epochs.
type Metadata struct {
	BaseHash     string `json:"base_hash"`
	BaseHeight   int64  `json:"base_height"`
	CoinsWritten int64  `json:"coins_written"`
	NChainTx     int64  `json:"nchaintx"`
	TxoutsetHash string `json:"txoutset_hash"`
	FileSha256   string `json:"file_sha256"`
	FileSize     int64  `json:"file_size"`
	CreatedAt    string `json:"created_at"`
}

type Server struct {
	client *http.Client

	mu         sync.Mutex
	refreshing bool
}

func snapshotDir() string  { return filepath.Join(storageDirectory, snapshotDirName) }
func snapshotPath() string { return filepath.Join(snapshotDir(), snapshotFileName) }
func metadataPath() string { return filepath.Join(snapshotDir(), metadataFileName) }

func getCredentials() (string, string, error) {
	rpcUser, err := os.ReadFile(filepath.Join(storageDirectory, "rpcuser.txt"))
	if err != nil {
		return "", "", err
	}
	rpcPassword, err := os.ReadFile(filepath.Join(storageDirectory, "rpcpassword.txt"))
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(rpcUser)), strings.TrimSpace(string(rpcPassword)), nil
}

func (s *Server) rpcCall(method string, params []interface{}, result interface{}) error {
	user, pass, err := getCredentials()
	if err != nil {
		return fmt.Errorf("reading RPC credentials: %w", err)
	}

	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "1.0",
		ID:      "core-snapshot",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s:22555/", os.Getenv("DBX_PUP_IP"))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
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

// readVarint reads a Bitcoin serialize.h-style VARINT.
func readVarint(r io.ByteReader) (uint64, error) {
	var n uint64
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		n = (n << 7) | uint64(b&0x7f)
		if b&0x80 == 0 {
			return n, nil
		}
		n++
	}
}

type bytesCounter struct {
	r io.Reader
	b [1]byte
	n int64
}

func newBytesCounter(r io.Reader) *bytesCounter { return &bytesCounter{r: r} }

func (c *bytesCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *bytesCounter) ReadByte() (byte, error) {
	_, err := io.ReadFull(c, c.b[:])
	return c.b[0], err
}

func reverseHex(raw []byte) string {
	rev := make([]byte, len(raw))
	for i, b := range raw {
		rev[len(raw)-1-i] = b
	}
	return hex.EncodeToString(rev)
}

// verifySnapshot structurally walks a dumptxoutset v1 file, checks the
// header against the expected base hash and coin count, and returns the
// exact byte offset at which the last coin record ends. Any bytes after
// that offset (e.g. an appended txoutset_hash) must be stripped before
// handing the file to d2.
func verifySnapshot(path string, expectedBaseHash string, expectedCoins int64) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	br := newBytesCounter(f)

	// Header: magic(5) version(u16 LE) network-magic(4) base-blockhash(32)
	// coins-count(u64 LE) nchaintx(u32 LE).
	header := make([]byte, 5+2+4+32+8+4)
	if _, err := io.ReadFull(br, header); err != nil {
		return 0, fmt.Errorf("reading snapshot header: %w", err)
	}
	if !bytes.Equal(header[:5], snapshotMagic) {
		return 0, fmt.Errorf("bad snapshot magic bytes")
	}
	if version := binary.LittleEndian.Uint16(header[5:7]); version != snapshotVersion {
		return 0, fmt.Errorf("unsupported snapshot version %d", version)
	}
	// uint256 is serialized in raw internal byte order; RPC hex is reversed.
	baseHash := reverseHex(header[11:43])
	if !strings.EqualFold(baseHash, expectedBaseHash) {
		return 0, fmt.Errorf("snapshot base hash %s does not match expected %s", baseHash, expectedBaseHash)
	}
	coinsCount := binary.LittleEndian.Uint64(header[43:51])
	if int64(coinsCount) != expectedCoins {
		return 0, fmt.Errorf("snapshot coin count %d does not match RPC coins_written %d", coinsCount, expectedCoins)
	}

	outpoint := make([]byte, 32+4)
	for i := uint64(0); i < coinsCount; i++ {
		if _, err := io.ReadFull(br, outpoint); err != nil {
			return 0, fmt.Errorf("truncated snapshot at coin %d (outpoint): %w", i, err)
		}
		if _, err := readVarint(br); err != nil { // code: height*2 + coinbase
			return 0, fmt.Errorf("truncated snapshot at coin %d (code): %w", i, err)
		}
		if _, err := readVarint(br); err != nil { // compressed amount
			return 0, fmt.Errorf("truncated snapshot at coin %d (amount): %w", i, err)
		}
		nSize, err := readVarint(br) // CScriptCompressor size
		if err != nil {
			return 0, fmt.Errorf("truncated snapshot at coin %d (script size): %w", i, err)
		}
		var payload uint64
		switch {
		case nSize <= 1: // P2PKH (0), P2SH (1): 20-byte hash160
			payload = 20
		case nSize <= 5: // P2PK forms (2-5): 32 bytes stored
			payload = 32
		default: // raw script of nSize-6 bytes
			payload = nSize - 6
		}
		if _, err := io.CopyN(io.Discard, br, int64(payload)); err != nil {
			return 0, fmt.Errorf("truncated snapshot at coin %d (script payload): %w", i, err)
		}
	}

	return br.n, nil
}

func fileSha256(path string) (string, int64, error) {
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

// refresh produces a fresh, verified snapshot and publishes it atomically.
func (s *Server) refresh() error {
	s.mu.Lock()
	if s.refreshing {
		s.mu.Unlock()
		return fmt.Errorf("a snapshot refresh is already in progress")
	}
	s.refreshing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.refreshing = false
		s.mu.Unlock()
	}()

	if err := os.MkdirAll(snapshotDir(), 0o755); err != nil {
		return err
	}

	// Record the best block hash before dumping (SPEC §11.2 step 1): the
	// dump must be based on exactly this block or the handoff is aborted.
	var bestBlockHash string
	if err := s.rpcCall("getbestblockhash", nil, &bestBlockHash); err != nil {
		return fmt.Errorf("getbestblockhash: %w", err)
	}
	log.Printf("Best block hash before dump: %s", bestBlockHash)

	// dumptxoutset refuses to overwrite; dump to a unique temp name
	// relative to the datadir (/storage), then rename into place.
	tempName := fmt.Sprintf("%s/%s.new-%d", snapshotDirName, snapshotFileName, time.Now().Unix())
	tempPath := filepath.Join(storageDirectory, tempName)
	defer os.Remove(tempPath)

	var dump dumpResult
	if err := s.rpcCall("dumptxoutset", []interface{}{tempName}, &dump); err != nil {
		return fmt.Errorf("dumptxoutset: %w", err)
	}
	log.Printf("dumptxoutset wrote %d coins at height %d (base %s)", dump.CoinsWritten, dump.BaseHeight, dump.BaseHash)

	// The dump must be based on the block we recorded beforehand;
	// otherwise the chain advanced mid-handoff and d2 would silently
	// accept the wrong base (it performs no base-blockhash check itself).
	if !strings.EqualFold(dump.BaseHash, bestBlockHash) {
		return fmt.Errorf("dump base hash %s does not match pre-dump best block %s; retry", dump.BaseHash, bestBlockHash)
	}

	// Structurally verify the file and locate the end of the last coin.
	endOffset, err := verifySnapshot(tempPath, dump.BaseHash, dump.CoinsWritten)
	if err != nil {
		return fmt.Errorf("verifying snapshot: %w", err)
	}

	info, err := os.Stat(tempPath)
	if err != nil {
		return err
	}
	if info.Size() < endOffset {
		return fmt.Errorf("snapshot smaller (%d) than parsed size (%d)", info.Size(), endOffset)
	}
	if info.Size() > endOffset {
		// Trailing bytes (e.g. an appended txoutset_hash) make d2's parser
		// fail with a Trailing error: strip them.
		log.Printf("Stripping %d trailing bytes after the last coin record", info.Size()-endOffset)
		if err := os.Truncate(tempPath, endOffset); err != nil {
			return fmt.Errorf("stripping trailing bytes: %w", err)
		}
	}

	sha, size, err := fileSha256(tempPath)
	if err != nil {
		return err
	}

	meta := Metadata{
		BaseHash:     dump.BaseHash,
		BaseHeight:   dump.BaseHeight,
		CoinsWritten: dump.CoinsWritten,
		NChainTx:     dump.NChainTx,
		TxoutsetHash: dump.TxoutsetHash,
		FileSha256:   sha,
		FileSize:     size,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	// Publish atomically: file first, then the metadata that references it.
	if err := os.Rename(tempPath, snapshotPath()); err != nil {
		return err
	}
	metaTemp := metadataPath() + ".tmp"
	if err := os.WriteFile(metaTemp, metaBytes, 0o644); err != nil {
		return err
	}
	if err := os.Rename(metaTemp, metadataPath()); err != nil {
		return err
	}

	log.Printf("Published snapshot: base %s height %d, %d coins, %d bytes, sha256 %s",
		meta.BaseHash, meta.BaseHeight, meta.CoinsWritten, meta.FileSize, meta.FileSha256)
	return nil
}

func (s *Server) currentMetadata() (*Metadata, error) {
	data, err := os.ReadFile(metadataPath())
	if err != nil {
		return nil, err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(metadataPath())
	if err != nil {
		http.Error(w, "no snapshot available yet", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(snapshotPath()); err != nil {
		http.Error(w, "no snapshot available yet", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, snapshotPath())
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := s.refresh(); err != nil {
		log.Printf("On-demand snapshot refresh failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.handleMetadata(w, r)
}

// refreshIfDue creates the first snapshot, or a fresh one when the current
// snapshot is older than the fortnightly reset interval.
func (s *Server) refreshIfDue() {
	meta, err := s.currentMetadata()
	if err == nil {
		created, perr := time.Parse(time.RFC3339, meta.CreatedAt)
		if perr == nil && time.Since(created) < refreshInterval {
			return
		}
	}
	if err := s.refresh(); err != nil {
		log.Printf("Scheduled snapshot refresh failed: %v", err)
	}
}

func main() {
	log.Println("Sleeping to give dogecoind time to start..")
	time.Sleep(10 * time.Second)

	server := &Server{
		// dumptxoutset walks the whole UTXO set; allow it plenty of time.
		client: &http.Client{Timeout: 2 * time.Hour},
	}

	go func() {
		for {
			server.refreshIfDue()
			time.Sleep(time.Hour)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/snapshot/metadata.json", server.handleMetadata)
	mux.HandleFunc("/snapshot/utxo.dat", server.handleFile)
	mux.HandleFunc("/snapshot/refresh", server.handleRefresh)

	addr := ":" + listenPort
	log.Printf("core-snapshot service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
