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

// This monitor reports D2 pup status to the Dogebox GUI by polling the
// d2-node public read-tier JSON-RPC 2.0 interface (no auth required for
// d2_getInfo / d2_getHealth / d2_getValidatorSet) on the pup's RPC port
// (42070, see pup.nix and manifest.json).

const d2RPCURL = "http://127.0.0.1:42070/"

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

type getInfoResult struct {
	Network         string `json:"network"`
	ChainID         string `json:"chainId"`
	Height          int    `json:"height"`
	FinalizedHeight int    `json:"finalizedHeight"`
	Peers           int    `json:"peers"`
	Syncing         bool   `json:"syncing"`
}

type getHealthResult struct {
	Status string `json:"status"`
}

type getValidatorSetResult struct {
	Epoch int `json:"epoch"`
}

type NodeInfo struct {
	Status       string
	Chain        string
	Blocks       int
	Headers      int
	TestnetEpoch string
}

var client = &http.Client{Timeout: 10 * time.Second}

func d2RPCCall(method string, params []interface{}, result interface{}) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      "d2-monitor",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", d2RPCURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
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

func getNodeInfo() NodeInfo {
	info := NodeInfo{
		Status:       "Not running",
		Chain:        "d2-testnet",
		Blocks:       0,
		Headers:      0,
		TestnetEpoch: "n/a",
	}

	var nodeInfo getInfoResult
	if err := d2RPCCall("d2_getInfo", []interface{}{}, &nodeInfo); err != nil {
		log.Printf("Error calling d2_getInfo: %v", err)
		return info
	}

	if nodeInfo.Network != "" {
		info.Chain = fmt.Sprintf("d2-%s", nodeInfo.Network)
	}
	// Headers track the known chain tip; Blocks track finalized height.
	info.Headers = nodeInfo.Height
	info.Blocks = nodeInfo.FinalizedHeight

	if nodeInfo.Syncing {
		info.Status = "Syncing"
	} else {
		info.Status = "Running"
	}

	var health getHealthResult
	if err := d2RPCCall("d2_getHealth", []interface{}{}, &health); err != nil {
		log.Printf("Error calling d2_getHealth: %v", err)
	} else if health.Status != "" && health.Status != "ok" {
		info.Status = fmt.Sprintf("%s (%s)", info.Status, health.Status)
	}

	var validatorSet getValidatorSetResult
	if err := d2RPCCall("d2_getValidatorSet", []interface{}{}, &validatorSet); err != nil {
		log.Printf("Error calling d2_getValidatorSet: %v", err)
	} else {
		info.TestnetEpoch = fmt.Sprintf("%d", validatorSet.Epoch)
	}

	return info
}

func submitMetrics(info NodeInfo) {
	jsonData := map[string]interface{}{
		"status":        map[string]interface{}{"value": info.Status},
		"chain":         map[string]interface{}{"value": info.Chain},
		"blocks":        map[string]interface{}{"value": info.Blocks},
		"headers":       map[string]interface{}{"value": info.Headers},
		"testnet_epoch": map[string]interface{}{"value": info.TestnetEpoch},
	}

	marshalledData, err := json.Marshal(jsonData)
	if err != nil {
		log.Printf("Error marshalling node info: %v", err)
		return
	}

	log.Printf("Submitting metrics: %v", jsonData)

	url := fmt.Sprintf("http://%s:%s/dbx/metrics", os.Getenv("DBX_HOST"), os.Getenv("DBX_PORT"))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(marshalledData))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending metrics: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Unexpected status code when submitting metrics: %d", resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Response body: %s", string(body))
		return
	}

	log.Println("Metrics submitted successfully.")
}

func main() {
	log.Println("Sleeping to give the D2 service time to start..")
	time.Sleep(10 * time.Second)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			info := getNodeInfo()
			submitMetrics(info)
		}
	}
}
