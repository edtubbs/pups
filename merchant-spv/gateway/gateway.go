package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var gatewayScript string
var storagePath string

type PaymentAddress struct {
	Address   string    `json:"address"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

type PaymentStatus struct {
	TxID          string  `json:"txid"`
	Address       string  `json:"address"`
	Amount        float64 `json:"amount"`
	Confirmations int     `json:"confirmations"`
	BlockHeight   int     `json:"block_height"`
}

type GatewayState struct {
	mu        sync.RWMutex
	addresses map[string]PaymentAddress
	pending   []PaymentStatus
}

var state = &GatewayState{
	addresses: make(map[string]PaymentAddress),
	pending:   []PaymentStatus{},
}

func executeGatewayCommand(args ...string) (string, error) {
	cmd := exec.Command(gatewayScript, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gateway command failed: %w, output: %s", err, output)
	}
	return string(output), nil
}

func generateNewAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Label string `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Label == "" {
		req.Label = "payment"
	}

	output, err := executeGatewayCommand("generateAddress", req.Label)
	if err != nil {
		log.Printf("Address generation failed: %v", err)
		http.Error(w, "Failed to generate address", http.StatusInternalServerError)
		return
	}

	var payAddr PaymentAddress
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &payAddr); err != nil {
		log.Printf("Failed to parse address response: %v", err)
		http.Error(w, "Failed to parse address", http.StatusInternalServerError)
		return
	}

	state.mu.Lock()
	state.addresses[payAddr.Address] = payAddr
	state.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payAddr)
}

func listAddresses(w http.ResponseWriter, r *http.Request) {
	output, err := executeGatewayCommand("listAddresses")
	if err != nil {
		log.Printf("List addresses failed: %v", err)
		http.Error(w, "Failed to list addresses", http.StatusInternalServerError)
		return
	}

	var addrs []PaymentAddress
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &addrs); err != nil {
		log.Printf("Failed to parse addresses: %v", err)
		http.Error(w, "Failed to parse addresses", http.StatusInternalServerError)
		return
	}

	// Update internal state
	state.mu.Lock()
	for _, addr := range addrs {
		state.addresses[addr.Address] = addr
	}
	state.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(addrs)
}

func checkPayments(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.pending)
}

func broadcastTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Hex string `json:"hex"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	output, err := executeGatewayCommand("broadcastTransaction", req.Hex)
	if err != nil {
		log.Printf("Broadcast failed: %v", err)
		http.Error(w, "Failed to broadcast transaction", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "broadcasted",
		"output": strings.TrimSpace(output),
	})
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "operational",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func monitorWalletDatabase() {
	walletPath := fmt.Sprintf("%s/merchant_wallet.db", storagePath)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Check if wallet database exists
		if _, err := os.Stat(walletPath); os.IsNotExist(err) {
			log.Printf("Wallet database not found at %s, waiting...", walletPath)
			continue
		}

		log.Printf("Wallet database found and monitoring...")
		// In a full implementation, we would query the wallet database
		// or use spvnode's wallet features to track transactions
	}
}

func main() {
	log.Println("Starting merchant payment gateway with libdogecoin spvnode...")
	log.Printf("Gateway script: %s", gatewayScript)
	log.Printf("Storage path: %s", storagePath)

	time.Sleep(10 * time.Second)

	go monitorWalletDatabase()

	http.HandleFunc("/api/address/new", generateNewAddress)
	http.HandleFunc("/api/address/list", listAddresses)
	http.HandleFunc("/api/payments", checkPayments)
	http.HandleFunc("/api/transaction/broadcast", broadcastTransaction)
	http.HandleFunc("/health", healthCheck)

	log.Println("Gateway API listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
