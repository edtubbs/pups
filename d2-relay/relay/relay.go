package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// d2-relay monitors UTXOs sent or received in each Dogecoin (D1) block and
// relays them to the D2 testnet. It also detects double spends (conflicting
// spends of the same outpoint) and reports testnet metrics to the Dogebox GUI.
//
// The D1 side reads confirmed transactions from core RPC and forwards their
// raw transaction bytes to the D2 node over d2-rpc.

// These match the temporary static credentials written by the core pup.
const (
	d1RPCUser = "dogebox_core_pup_temporary_static_username"
	d1RPCPass = "dogebox_core_pup_temporary_static_password"
)

// maxTrackedOutpoints bounds the in-memory spent-outpoint set used for
// double-spend detection.
const maxTrackedOutpoints = 1000000

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

type Vin struct {
	TxID     string `json:"txid"`
	Vout     int    `json:"vout"`
	Coinbase string `json:"coinbase"`
}

type Vout struct {
	Value float64 `json:"value"`
	N     int     `json:"n"`
}

type Tx struct {
	TxID string `json:"txid"`
	Hex  string `json:"hex"`
	Vin  []Vin  `json:"vin"`
	Vout []Vout `json:"vout"`
}

type Block struct {
	Hash   string `json:"hash"`
	Height int    `json:"height"`
	Tx     []Tx   `json:"tx"`
}

type Relay struct {
	client   *http.Client
	d1RPCURL string
	d2RPCURL string

	d2BearerToken string

	spentOutpoints map[string]string // outpoint -> spending txid

	// Metrics
	d1Height     int
	lastBlock    string
	utxosCreated int
	utxosSpent   int
	relayedTxs   int
	doubleSpends int
}

func newRelay() *Relay {
	d1Host := os.Getenv("DBX_IFACE_CORE_RPC_HOST")
	d1Port := os.Getenv("DBX_IFACE_CORE_RPC_PORT")
	d2Host := os.Getenv("DBX_IFACE_D2_RPC_HOST")
	d2Port := os.Getenv("DBX_IFACE_D2_RPC_PORT")

	return &Relay{
		client:        &http.Client{Timeout: 30 * time.Second},
		d1RPCURL:      fmt.Sprintf("http://%s:%s/", d1Host, d1Port),
		d2RPCURL:      fmt.Sprintf("http://%s:%s/", d2Host, d2Port),
		d2BearerToken: strings.TrimSpace(firstNonEmptyEnv("DBX_IFACE_D2_RPC_BEARER_TOKEN", "D2_RPC_BEARER_TOKEN")),

		spentOutpoints: make(map[string]string),
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func (r *Relay) d1RPCCall(method string, params []interface{}, result interface{}) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "1.0",
		ID:      "d2-relay",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", r.d1RPCURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.SetBasicAuth(d1RPCUser, d1RPCPass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
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

func (r *Relay) d2RPCCall(method string, params []interface{}, result interface{}) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      "d2-relay",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", r.d2RPCURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.d2BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.d2BearerToken)
	}

	resp, err := r.client.Do(req)
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

func (r *Relay) getBlockCount() (int, error) {
	var count int
	err := r.d1RPCCall("getblockcount", nil, &count)
	return count, err
}

func (r *Relay) getBlockHash(height int) (string, error) {
	var hash string
	err := r.d1RPCCall("getblockhash", []interface{}{height}, &hash)
	return hash, err
}

func (r *Relay) getBlock(hash string) (Block, error) {
	var block Block
	// Verbosity 2 returns full transaction objects.
	err := r.d1RPCCall("getblock", []interface{}{hash, 2}, &block)
	return block, err
}

func (r *Relay) getRawTransactionHex(txid string, blockHash string) (string, error) {
	var hex string
	err := r.d1RPCCall("getrawtransaction", []interface{}{txid, false, blockHash}, &hex)
	return hex, err
}

func (r *Relay) relayToD2(tx Tx, blockHash string) error {
	rawHex := strings.TrimSpace(tx.Hex)
	if rawHex == "" {
		fallbackHex, err := r.getRawTransactionHex(tx.TxID, blockHash)
		if err != nil {
			return fmt.Errorf("missing tx hex in block payload and getrawtransaction fallback failed: %w", err)
		}
		rawHex = strings.TrimSpace(fallbackHex)
	}

	if rawHex == "" {
		return fmt.Errorf("empty raw tx hex for tx %s", tx.TxID)
	}

	var result struct {
		TxID string `json:"txid"`
	}
	if err := r.d2RPCCall("d2_sendRawTransaction", []interface{}{rawHex}, &result); err != nil {
		return fmt.Errorf("failed to relay tx %s to D2: %w", tx.TxID, err)
	}

	if result.TxID == "" {
		return fmt.Errorf("d2_sendRawTransaction returned empty txid for source tx %s", tx.TxID)
	}

	if result.TxID != tx.TxID {
		log.Printf("Warning: D2 txid mismatch for tx %s, returned %s", tx.TxID, result.TxID)
	}

	return nil
}

func (r *Relay) processBlock(block Block) {
	log.Printf("Processing D1 block %d (%s) with %d transactions", block.Height, block.Hash, len(block.Tx))

	for _, tx := range block.Tx {
		for _, vin := range tx.Vin {
			if vin.Coinbase != "" {
				continue
			}

			outpoint := fmt.Sprintf("%s:%d", vin.TxID, vin.Vout)
			if spender, seen := r.spentOutpoints[outpoint]; seen {
				if spender != tx.TxID {
					r.doubleSpends++
					log.Printf("DOUBLE SPEND detected: outpoint %s spent by both %s and %s", outpoint, spender, tx.TxID)
				}
				// Duplicate input within the same transaction: skip.
			} else {
				r.spentOutpoints[outpoint] = tx.TxID
				r.utxosSpent++
			}
		}

		r.utxosCreated += len(tx.Vout)

		if err := r.relayToD2(tx, block.Hash); err != nil {
			log.Printf("Error relaying tx %s to D2: %v", tx.TxID, err)
			continue
		}
		r.relayedTxs++
	}

	// Bound memory usage: reset tracking if the set grows too large.
	if len(r.spentOutpoints) > maxTrackedOutpoints {
		log.Printf("Spent outpoint set exceeded %d entries, resetting", maxTrackedOutpoints)
		r.spentOutpoints = make(map[string]string)
	}

	r.d1Height = block.Height
	r.lastBlock = block.Hash
}

func (r *Relay) submitMetrics() {
	jsonData := map[string]interface{}{
		"d1_height":     map[string]interface{}{"value": r.d1Height},
		"last_block":    map[string]interface{}{"value": r.lastBlock},
		"utxos_created": map[string]interface{}{"value": r.utxosCreated},
		"utxos_spent":   map[string]interface{}{"value": r.utxosSpent},
		"relayed_txs":   map[string]interface{}{"value": r.relayedTxs},
		"double_spends": map[string]interface{}{"value": r.doubleSpends},
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

	resp, err := r.client.Do(req)
	if err != nil {
		log.Printf("Error sending metrics: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Unexpected status code when submitting metrics: %d, body: %s", resp.StatusCode, string(body))
		return
	}

	log.Println("Metrics submitted successfully.")
}

func main() {
	log.Println("Sleeping to give the D1 core node time to start..")
	time.Sleep(10 * time.Second)

	relay := newRelay()
	log.Printf("Relaying from D1 RPC at %s", relay.d1RPCURL)
	log.Printf("Relaying into D2 RPC at %s", relay.d2RPCURL)
	if relay.d2BearerToken == "" {
		log.Printf("Warning: D2 RPC bearer token env var is unset (checked DBX_IFACE_D2_RPC_BEARER_TOKEN and D2_RPC_BEARER_TOKEN); relay will call d2_sendRawTransaction without Authorization and may receive unauthorized errors")
	}

	nextHeight := -1

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			count, err := relay.getBlockCount()
			if err != nil {
				log.Printf("Error getting D1 block count: %v", err)
				continue
			}

			if nextHeight < 0 {
				// Start relaying from the current chain tip.
				nextHeight = count
			}

			for nextHeight <= count {
				hash, err := relay.getBlockHash(nextHeight)
				if err != nil {
					log.Printf("Error getting block hash at height %d: %v", nextHeight, err)
					break
				}

				block, err := relay.getBlock(hash)
				if err != nil {
					log.Printf("Error getting block %s: %v", hash, err)
					break
				}

				relay.processBlock(block)
				nextHeight++
			}

			relay.submitMetrics()
		}
	}
}
