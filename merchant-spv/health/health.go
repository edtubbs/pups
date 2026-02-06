package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var binaryPath string

type ChainStats struct {
	NetworkName    string  `json:"chain"`
	CurrentBlocks  int     `json:"blocks"`
	HeadersTotal   int     `json:"headers"`
	NetworkDiff    float64 `json:"difficulty"`
	SyncPercent    float64 `json:"verification_progress"`
	InitialSync    bool    `json:"initial_block_download"`
	DiskUsageBytes int64   `json:"size_on_disk"`
}

type StatusUpdate struct {
	mu    sync.RWMutex
	stats ChainStats
	ready bool
}

var globalStatus = &StatusUpdate{}

func retrieveAuthCredentials() (string, string, error) {
	userBytes, err := os.ReadFile("/storage/rpcuser.txt")
	if err != nil {
		return "", "", fmt.Errorf("failed reading user: %w", err)
	}

	passBytes, err := os.ReadFile("/storage/rpcpassword.txt")
	if err != nil {
		return "", "", fmt.Errorf("failed reading password: %w", err)
	}

	return strings.TrimSpace(string(userBytes)), strings.TrimSpace(string(passBytes)), nil
}

func queryNodeStatus(username, password string) (ChainStats, error) {
	cliTool := fmt.Sprintf("%s/bin/dogecoin-cli", binaryPath)
	
	args := []string{
		fmt.Sprintf("-rpcuser=%s", username),
		fmt.Sprintf("-rpcpassword=%s", password),
		fmt.Sprintf("-rpcconnect=%s", os.Getenv("DBX_PUP_IP")),
		"getblockchaininfo",
	}

	cmd := exec.Command(cliTool, args...)
	cmd.Env = append(os.Environ(), "HOME=/tmp")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ChainStats{}, fmt.Errorf("cli execution failed: %w", err)
	}

	var stats ChainStats
	if err := json.Unmarshal(output, &stats); err != nil {
		return ChainStats{}, fmt.Errorf("json parse error: %w", err)
	}

	return stats, nil
}

func transmitMetrics(stats ChainStats) {
	metricsPayload := map[string]interface{}{
		"network":         map[string]interface{}{"value": stats.NetworkName},
		"current_height":  map[string]interface{}{"value": stats.CurrentBlocks},
		"header_count":    map[string]interface{}{"value": stats.HeadersTotal},
		"awaiting_confirm": map[string]interface{}{"value": 0},
		"daily_confirmed":  map[string]interface{}{"value": 0},
	}

	jsonPayload, err := json.Marshal(metricsPayload)
	if err != nil {
		log.Printf("Metrics encoding failed: %v", err)
		return
	}

	endpoint := fmt.Sprintf("http://%s:%s/dbx/metrics", 
		os.Getenv("DBX_HOST"), 
		os.Getenv("DBX_PORT"))

	resp, err := http.Post(endpoint, "application/json", strings.NewReader(string(jsonPayload)))
	if err != nil {
		log.Printf("Metrics transmission failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Metrics rejected (status=%d): %s", resp.StatusCode, body)
		return
	}

	log.Println("Metrics transmitted successfully")
}

func monitorLoop() {
	log.Println("Waiting for node initialization...")
	time.Sleep(12 * time.Second)

	username, password, err := retrieveAuthCredentials()
	if err != nil {
		log.Fatalf("Credential retrieval failed: %v", err)
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats, err := queryNodeStatus(username, password)
		if err != nil {
			log.Printf("Status query failed: %v", err)
			continue
		}

		globalStatus.mu.Lock()
		globalStatus.stats = stats
		globalStatus.ready = true
		globalStatus.mu.Unlock()

		log.Printf("Network: %s | Blocks: %d | Headers: %d | Sync: %.2f%%",
			stats.NetworkName, stats.CurrentBlocks, stats.HeadersTotal, stats.SyncPercent*100)

		transmitMetrics(stats)
	}
}

func main() {
	go monitorLoop()
	select {}
}
