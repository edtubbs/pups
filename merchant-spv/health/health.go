package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var libdogecoinPath string
var storagePath string

type SyncStatus struct {
	HeadersFile   string `json:"headers_file"`
	WalletFile    string `json:"wallet_file"`
	SyncCompleted bool   `json:"sync_completed"`
}

func checkSPVNodeStatus() (SyncStatus, error) {
	headersPath := fmt.Sprintf("%s/headers.db", storagePath)
	walletPath := fmt.Sprintf("%s/merchant_wallet.db", storagePath)

	status := SyncStatus{
		HeadersFile:   headersPath,
		WalletFile:    walletPath,
		SyncCompleted: false,
	}

	// Check if files exist
	if _, err := os.Stat(headersPath); err == nil {
		status.SyncCompleted = true
	}

	return status, nil
}

func submitMetrics(status SyncStatus) {
	metricsPayload := map[string]interface{}{
		"network":          map[string]interface{}{"value": "mainnet"},
		"current_height":   map[string]interface{}{"value": 0},
		"header_count":     map[string]interface{}{"value": 0},
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

	resp, err := http.Post(endpoint, "application/json", string(jsonPayload))
	if err != nil {
		log.Printf("Metrics transmission failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Metrics rejected (status=%d)", resp.StatusCode)
		return
	}

	log.Println("Metrics transmitted successfully")
}

func monitorLoop() {
	log.Println("Waiting for spvnode initialization...")
	time.Sleep(15 * time.Second)

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		status, err := checkSPVNodeStatus()
		if err != nil {
			log.Printf("Status check failed: %v", err)
			continue
		}

		log.Printf("SPV Node Status | Headers DB: %s | Wallet DB: %s | Synced: %v",
			status.HeadersFile, status.WalletFile, status.SyncCompleted)

		submitMetrics(status)
	}
}

func main() {
	go monitorLoop()
	select {}
}
