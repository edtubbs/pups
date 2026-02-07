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

var storageDirectory = "/storage"

type Metrics struct {
    Mnemonic         string `json:"mnemonic"`
    Chaintip         string `json:"chaintip"`
    Balance          string `json:"balance"`
    Addresses        string `json:"addresses"`
    TransactionCount string `json:"transaction_count"`
    UnspentCount     string `json:"unspent_count"`
    Transactions     string `json:"transactions"`
    UTXOs            string `json:"utxos"`
}

func fetchEndpoint(endpoint string) (string, error) {
    url := fmt.Sprintf("http://0.0.0.0:8888%s", endpoint)

    client := &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            DisableKeepAlives: true,
        },
    }

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return "", fmt.Errorf("error creating request: %w", err)
    }

    req.Header.Set("Connection", "close")

    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("error sending request to %s: %w", endpoint, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("unexpected status code %d for %s: %s", resp.StatusCode, endpoint, string(body))
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error reading response body for %s: %w", endpoint, err)
    }

    return string(body), nil
}

// readMnemonic reads the mnemonic from the MNEMONIC_PHRASE environment variable
// Returns the mnemonic on first read, then marks as viewed and returns a message
func readMnemonic() string {
    // Check if already viewed via environment variable
    if os.Getenv("MNEMONIC_VIEWED") == "true" {
        return "[Mnemonic was displayed and should have been saved]"
    }
    
    // Read mnemonic directly from environment variable
    mnemonic := os.Getenv("MNEMONIC_PHRASE")
    
    // If not set yet, check if wallet is being initialized
    if mnemonic == "" {
        // Check if wallet.db exists to determine state
        walletDbFile := storageDirectory + "/wallet.db"
        if _, err := os.Stat(walletDbFile); os.IsNotExist(err) {
            return "[Waiting for wallet initialization...]"
        }
        // Wallet exists but mnemonic not in env - already been cleared
        return "[Mnemonic was displayed and should have been saved]"
    }
    
    // Return the mnemonic (will be marked as viewed after successful submission)
    return mnemonic
}

func collectMetrics() (Metrics, error) {
    var metrics Metrics

    // Read mnemonic for one-time display
    metrics.Mnemonic = readMnemonic()

    // Fetch chain tip
    chaintipStr, err := fetchEndpoint("/getChaintip")
    if err != nil {
        return metrics, err
    }
    metrics.Chaintip = parseSimpleMetric(chaintipStr, "Chain tip: ")

    // Fetch balance
    balanceStr, err := fetchEndpoint("/getBalance")
    if err != nil {
        return metrics, err
    }
    balance := parseSimpleMetric(balanceStr, "Wallet balance: ")
    metrics.Balance = fmt.Sprintf("%sÐ", balance)

    // Fetch addresses
    addressesStr, err := fetchEndpoint("/getAddresses")
    if err != nil {
        return metrics, err
    }
    metrics.Addresses = parseListMetric(addressesStr, "address: ")

    // Fetch transactions
    transactionsStr, err := fetchEndpoint("/getTransactions")
    if err != nil {
        return metrics, err
    }
    metrics.Transactions, metrics.TransactionCount = parseUTXOsOrTxs(transactionsStr)

    // Fetch UTXOs
    utxosStr, err := fetchEndpoint("/getUTXOs")
    if err != nil {
        return metrics, err
    }
    metrics.UTXOs, metrics.UnspentCount = parseUTXOsOrTxs(utxosStr)

    return metrics, nil
}

// Helper to parse simple metrics
func parseSimpleMetric(input, prefix string) string {
    line := strings.TrimSpace(input)
    if strings.HasPrefix(line, prefix) {
        return strings.TrimPrefix(line, prefix)
    }
    return line
}

// Helper to parse lists of metrics
func parseListMetric(input, prefix string) string {
    var items []string
    for _, line := range strings.Split(input, "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, prefix) {
            items = append(items, strings.TrimPrefix(line, prefix))
        }
    }
    if len(items) > 0 {
        return strings.Join(items, "\n")
    }
    return "No entries found"
}

// Helper to parse UTXOs or transactions
func parseUTXOsOrTxs(input string) (string, string) {
    var output []string
    count := 0

    parts := strings.Split(input, "----------------------")
    for _, part := range parts {
        part = strings.TrimSpace(part)
        if part == "" {
            continue
        }

        txid, amount, address := "", "", ""
        lines := strings.Split(part, "\n")
        for _, line := range lines {
            line = strings.TrimSpace(line)
            if strings.HasPrefix(line, "txid:") {
                txid = strings.TrimSpace(strings.TrimPrefix(line, "txid:"))
            } else if strings.HasPrefix(line, "amount:") {
                amount = strings.TrimSpace(strings.TrimPrefix(line, "amount:"))
            } else if strings.HasPrefix(line, "address:") {
                address = strings.TrimSpace(strings.TrimPrefix(line, "address:"))
            }
        }

        if txid != "" && amount != "" && address != "" {
            output = append(output, fmt.Sprintf("%s %sÐ %s", txid, amount, address))
            count++
        }
    }

    return strings.Join(output, "\n"), fmt.Sprintf("%d", count)
}

func submitMetrics(metrics Metrics) {
    client := &http.Client{
        Timeout: 10 * time.Second,
    }

    jsonData := map[string]interface{}{
        "mnemonic":          map[string]interface{}{"value": metrics.Mnemonic},
        "chaintip":          map[string]interface{}{"value": metrics.Chaintip},
        "balance":           map[string]interface{}{"value": metrics.Balance},
        "addresses":         map[string]interface{}{"value": metrics.Addresses},
        "transaction_count": map[string]interface{}{"value": metrics.TransactionCount},
        "unspent_count":     map[string]interface{}{"value": metrics.UnspentCount},
        "transactions":      map[string]interface{}{"value": metrics.Transactions},
        "utxos":             map[string]interface{}{"value": metrics.UTXOs},
    }

    marshalledData, err := json.Marshal(jsonData)
    if err != nil {
        log.Printf("Error marshalling metrics: %v", err)
        return
    }

    url := fmt.Sprintf("http://%s:%s/dbx/metrics", os.Getenv("DBX_HOST"), os.Getenv("DBX_PORT"))

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(marshalledData))
    if err != nil {
        log.Printf("Error creating request: %v", err)
        return
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Connection", "close")

    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Error sending metrics: %v", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        log.Printf("Unexpected status code when submitting metrics: %d", resp.StatusCode)
        log.Printf("Response body: %s", string(body))
        return
    }

    // After successful submission, mark mnemonic as viewed if it was just displayed
    markMnemonicAsViewed(metrics.Mnemonic)
}

// markMnemonicAsViewed marks the mnemonic as viewed by setting an environment variable
func markMnemonicAsViewed(mnemonic string) {
    // Only mark as viewed if we actually sent a real mnemonic (not a status message)
    if !strings.HasPrefix(mnemonic, "[") {
        // Set environment variable to mark as viewed
        if err := os.Setenv("MNEMONIC_VIEWED", "true"); err != nil {
            log.Printf("Error setting MNEMONIC_VIEWED environment variable: %v", err)
        } else {
            log.Println("Mnemonic displayed successfully - marked as viewed via environment variable")
        }
    }
}

func main() {
    log.Println("Sleeping to give spvnode time to start...")
    time.Sleep(10 * time.Second)

    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            metrics, err := collectMetrics()
            if err != nil {
                log.Printf("Error collecting metrics: %v", err)
                continue
            }

            log.Printf("Metrics: %+v", metrics)
            submitMetrics(metrics)
            log.Printf("----------------------------------------")
        }
    }
}
