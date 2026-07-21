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

// This monitor is scaffolding for the D2 testnet pup. Once the D2 node
// binary is available, it should query the node's RPC interface (see the
// core pup's monitor for the pattern) and report real chain metrics.
// Until then it reports a static "waiting" status so the Dogebox GUI shows
// the pup state clearly.

type NodeInfo struct {
	Status       string
	Chain        string
	Blocks       int
	Headers      int
	TestnetEpoch string
}

// getNodeInfo will query the D2 node RPC once the binary is available.
// TODO: Implement real RPC calls against the D2 node (rpc port 42070,
// credentials in /storage/rpcuser.txt and /storage/rpcpassword.txt).
func getNodeInfo() NodeInfo {
	return NodeInfo{
		Status:       "Waiting for D2 release",
		Chain:        "d2-testnet",
		Blocks:       0,
		Headers:      0,
		TestnetEpoch: "n/a",
	}
}

func submitMetrics(info NodeInfo) {
	client := &http.Client{}

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
