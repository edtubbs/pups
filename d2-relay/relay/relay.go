package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// d2-relay monitors UTXOs sent or received in each Dogecoin (D1) block and
// relays them to the D2 testnet. It also detects double spends (conflicting
// spends of the same outpoint) and reports testnet metrics to the Dogebox GUI.
//
// The D1 side is fully implemented against the core pup's RPC interface.
// The D2 submission side is stubbed until a D2 release artifact is available
// (see relayToD2 below).

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
	Vin  []Vin  `json:"vin"`
	Vout []Vout `json:"vout"`
}

type Block struct {
	Hash   string `json:"hash"`
	Height int    `json:"height"`
	Tx     []Tx   `json:"tx"`
}

type Relay struct {
	client *http.Client
	rpcURL string

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
	host := os.Getenv("DBX_IFACE_CORE_RPC_HOST")
	port := os.Getenv("DBX_IFACE_CORE_RPC_PORT")

	return &Relay{
		client:         &http.Client{Timeout: 30 * time.Second},
		rpcURL:         fmt.Sprintf("http://%s:%s/", host, port),
		spentOutpoints: make(map[string]string),
	}
}

func (r *Relay) rpcCall(method string, params []interface{}, result interface{}) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "1.0",
		ID:      "d2-relay",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", r.rpcURL, bytes.NewBuffer(reqBody))
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

func (r *Relay) getBlockCount() (int, error) {
	var count int
	err := r.rpcCall("getblockcount", nil, &count)
	return count, err
}

func (r *Relay) getBlockHash(height int) (string, error) {
	var hash string
	err := r.rpcCall("getblockhash", []interface{}{height}, &hash)
	return hash, err
}

func (r *Relay) getBlock(hash string) (Block, error) {
	var block Block
	// Verbosity 2 returns full transaction objects.
	err := r.rpcCall("getblock", []interface{}{hash, 2}, &block)
	return block, err
}

// relayToD2 submits a D1 transaction's UTXO activity to the D2 testnet.
//
// TODO: Implement once the D2 pup has a real node. This should use the
// d2-rpc interface (DBX_IFACE_D2_RPC_HOST / DBX_IFACE_D2_RPC_PORT) to mirror
// the transaction on the D2 testnet for stress-testing.
func (r *Relay) relayToD2(tx Tx) error {
	log.Printf("[stub] would relay tx %s to D2 (%d inputs, %d outputs)", tx.TxID, len(tx.Vin), len(tx.Vout))
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

		if err := r.relayToD2(tx); err != nil {
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
	log.Printf("Relaying from D1 RPC at %s", relay.rpcURL)

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
