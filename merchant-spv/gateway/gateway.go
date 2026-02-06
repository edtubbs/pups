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

var libdogecoinPath string
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

func generateAddress(label string) (string, error) {
	suchBin := fmt.Sprintf("%s/bin/such", libdogecoinPath)
	
	// Generate new private key
	privKeyCmd := exec.Command(suchBin, "-c", "generate_private_key")
	privOutput, err := privKeyCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to generate key: %w, output: %s", err, privOutput)
	}

	// Parse private key WIF
	lines := strings.Split(string(privOutput), "\n")
	var privKey string
	for _, line := range lines {
		if strings.Contains(line, "privatekey WIF:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				privKey = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	if privKey == "" {
		return "", fmt.Errorf("could not parse private key from output")
	}

	// Generate public key and address
	pubKeyCmd := exec.Command(suchBin, "-c", "generate_public_key", "-p", privKey)
	pubOutput, err := pubKeyCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to generate address: %w", err)
	}

	// Parse address
	lines = strings.Split(string(pubOutput), "\n")
	var address string
	for _, line := range lines {
		if strings.Contains(line, "p2pkh address:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				address = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	if address == "" {
		return "", fmt.Errorf("could not parse address from output")
	}

	return address, nil
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

	address, err := generateAddress(req.Label)
	if err != nil {
		log.Printf("Address generation failed: %v", err)
		http.Error(w, "Failed to generate address", http.StatusInternalServerError)
		return
	}

	payAddr := PaymentAddress{
		Address:   address,
		Label:     req.Label,
		CreatedAt: time.Now(),
	}

	state.mu.Lock()
	state.addresses[address] = payAddr
	state.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payAddr)
}

func listAddresses(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	addrs := make([]PaymentAddress, 0, len(state.addresses))
	for _, addr := range state.addresses {
		addrs = append(addrs, addr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(addrs)
}

func checkPayments(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.pending)
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
		// In a full implementation, we would query the libdogecoin wallet database
		// for transactions related to our monitored addresses
	}
}

func main() {
	log.Println("Starting merchant payment gateway for libdogecoin spvnode...")
	time.Sleep(10 * time.Second)

	go monitorWalletDatabase()

	http.HandleFunc("/api/address/new", generateNewAddress)
	http.HandleFunc("/api/address/list", listAddresses)
	http.HandleFunc("/api/payments", checkPayments)
	http.HandleFunc("/health", healthCheck)

	log.Println("Gateway API listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
