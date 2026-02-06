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

var cliPath string

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
	Timestamp     int64   `json:"timestamp"`
}

type GatewayState struct {
	mu       sync.RWMutex
	addresses map[string]PaymentAddress
	pending   []PaymentStatus
}

var state = &GatewayState{
	addresses: make(map[string]PaymentAddress),
	pending:   []PaymentStatus{},
}

func loadAuthData() (string, string, error) {
	userContent, err := os.ReadFile("/storage/rpcuser.txt")
	if err != nil {
		return "", "", err
	}

	passContent, err := os.ReadFile("/storage/rpcpassword.txt")
	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(string(userContent)), strings.TrimSpace(string(passContent)), nil
}

func executeRPCCall(user, pass, method string, params ...interface{}) ([]byte, error) {
	cliExec := fmt.Sprintf("%s/bin/dogecoin-cli", cliPath)
	
	cmdArgs := []string{
		fmt.Sprintf("-rpcuser=%s", user),
		fmt.Sprintf("-rpcpassword=%s", pass),
		fmt.Sprintf("-rpcconnect=%s", os.Getenv("DBX_PUP_IP")),
		"-rpcwallet=payments",
		method,
	}

	for _, param := range params {
		cmdArgs = append(cmdArgs, fmt.Sprintf("%v", param))
	}

	cmd := exec.Command(cliExec, cmdArgs...)
	cmd.Env = append(os.Environ(), "HOME=/tmp")

	return cmd.CombinedOutput()
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

	user, pass, err := loadAuthData()
	if err != nil {
		http.Error(w, "Authentication error", http.StatusInternalServerError)
		return
	}

	output, err := executeRPCCall(user, pass, "getnewaddress", req.Label)
	if err != nil {
		log.Printf("Address generation failed: %v", err)
		http.Error(w, "Failed to generate address", http.StatusInternalServerError)
		return
	}

	address := strings.TrimSpace(string(output))

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

func monitorIncomingPayments() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		user, pass, err := loadAuthData()
		if err != nil {
			log.Printf("Auth error in monitor: %v", err)
			continue
		}

		output, err := executeRPCCall(user, pass, "listtransactions", "*", 50)
		if err != nil {
			log.Printf("Transaction list error: %v", err)
			continue
		}

		var transactions []map[string]interface{}
		if err := json.Unmarshal(output, &transactions); err != nil {
			log.Printf("Transaction parse error: %v", err)
			continue
		}

		state.mu.Lock()
		state.pending = []PaymentStatus{}
		for _, tx := range transactions {
			if tx["category"] == "receive" {
				status := PaymentStatus{
					TxID:          fmt.Sprintf("%v", tx["txid"]),
					Address:       fmt.Sprintf("%v", tx["address"]),
					Amount:        tx["amount"].(float64),
					Confirmations: int(tx["confirmations"].(float64)),
					Timestamp:     int64(tx["time"].(float64)),
				}
				state.pending = append(state.pending, status)
			}
		}
		state.mu.Unlock()

		log.Printf("Monitored %d incoming payments", len(state.pending))
	}
}

func main() {
	log.Println("Starting merchant payment gateway...")
	time.Sleep(15 * time.Second)

	go monitorIncomingPayments()

	http.HandleFunc("/api/address/new", generateNewAddress)
	http.HandleFunc("/api/address/list", listAddresses)
	http.HandleFunc("/api/payments", checkPayments)
	http.HandleFunc("/health", healthCheck)

	log.Println("Gateway API listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
