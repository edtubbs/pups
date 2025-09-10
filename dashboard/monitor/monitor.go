package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"
)

const (
    statsBlocks    = 11
    statsWindowSec = 86400
)

type Metrics struct {
    Chaintip     string `json:"chaintip"`
    Balance      string `json:"balance"`
    Addresses    string `json:"addresses"`
    Transactions string `json:"transactions"`
    UTXOs        string `json:"utxos"`
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

func collectMetrics() (Metrics, map[string]string, map[string]string, error) {
    var metrics Metrics

    // Chain tip
    chaintipStr, err := fetchEndpoint("/getChaintip")
    if err != nil {
        return metrics, nil, nil, err
    }
    metrics.Chaintip = parseSimpleMetric(chaintipStr, "Chain tip: ")

    // Balance
    balanceStr, err := fetchEndpoint("/getBalance")
    if err != nil {
        return metrics, nil, nil, err
    }
    balance := parseSimpleMetric(balanceStr, "Wallet balance: ")
    metrics.Balance = fmt.Sprintf("%sÐ", balance)

    // Addresses
    addressesStr, err := fetchEndpoint("/getAddresses")
    if err != nil {
        return metrics, nil, nil, err
    }
    metrics.Addresses = parseListMetric(addressesStr, "address: ")

    // Transactions (spent)
    transactionsStr, err := fetchEndpoint("/getTransactions")
    if err != nil {
        return metrics, nil, nil, err
    }
    metrics.Transactions, _ = parseUTXOsOrTxs(transactionsStr)

    // UTXOs (unspent)
    utxosStr, err := fetchEndpoint("/getUTXOs")
    if err != nil {
        return metrics, nil, nil, err
    }
    metrics.UTXOs, _ = parseUTXOsOrTxs(utxosStr)

    // Stats over last N blocks
    statsStr, err := fetchEndpoint(fmt.Sprintf("/stats?blocks=%d&secs=%d", statsBlocks, statsWindowSec))
    if err != nil {
        return metrics, nil, nil, err
    }
    statsMap := parseKeyValueLines(statsStr)

    // Chain (session) stats
    chainStr, err := fetchEndpoint("/chainStats")
    if err != nil {
        return metrics, nil, nil, err
    }
    chainMap := parseKeyValueLines(chainStr)

    // SMPV stats (mempool + script-type totals). Not fatal if disabled.
    if smpvStr, err := fetchEndpoint("/smpvStats"); err == nil {
        for k, v := range parseKeyValueLines(smpvStr) {
            statsMap[k] = v // merge into stats namespace
        }
    }

    return metrics, statsMap, chainMap, nil
}

func parseSimpleMetric(input, prefix string) string {
    line := strings.TrimSpace(input)
    if strings.HasPrefix(line, prefix) {
        return strings.TrimPrefix(line, prefix)
    }
    return line
}

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
        for _, line := range strings.Split(part, "\n") {
            line = strings.TrimSpace(line)
            switch {
            case strings.HasPrefix(line, "txid:"):
                txid = strings.TrimSpace(strings.TrimPrefix(line, "txid:"))
            case strings.HasPrefix(line, "amount:"):
                amount = strings.TrimSpace(strings.TrimPrefix(line, "amount:"))
            case strings.HasPrefix(line, "address:"):
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

// Parses "key: value" lines into a map, ignores headings like "=== ... ==="
func parseKeyValueLines(input string) map[string]string {
    m := make(map[string]string)
    for _, line := range strings.Split(input, "\n") {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, "===") {
            continue
        }
        idx := strings.Index(line, ":")
        if idx <= 0 {
            continue
        }
        key := strings.TrimSpace(line[:idx])
        val := strings.TrimSpace(line[idx+1:])
        m[strings.ReplaceAll(strings.ToLower(key), " ", "_")] = val
    }
    return m
}

func submitMetrics(metrics Metrics, stats map[string]string, chain map[string]string) {
    client := &http.Client{Timeout: 10 * time.Second}

    payload := map[string]interface{}{
        // Basics
        "chaintip":     map[string]interface{}{"value": metrics.Chaintip},
        "balance":      map[string]interface{}{"value": metrics.Balance},
        "addresses":    map[string]interface{}{"value": metrics.Addresses},
        "transactions": map[string]interface{}{"value": metrics.Transactions},
        "utxos":        map[string]interface{}{"value": metrics.UTXOs},

        // /stats?blocks=11
        "stats_blocks":               map[string]interface{}{"value": mustParseFloat(stats["blocks"])},
        "stats_transactions":         map[string]interface{}{"value": mustParseFloat(stats["transactions"])},
        "stats_tps":                  map[string]interface{}{"value": mustParseFloat(stats["tps"])},
        "stats_volume":               map[string]interface{}{"value": mustParseFloat(stats["volume"])},
        "stats_volume_koinu":         map[string]interface{}{"value": mustParseFloat(stats["volume_koinu"])},
        "stats_median_fee_per_block": map[string]interface{}{"value": mustParseFloat(stats["median_fee_per_block"])},
        "stats_avg_fee_per_block":    map[string]interface{}{"value": mustParseFloat(stats["avg_fee_per_block"])},
        "stats_outputs":              map[string]interface{}{"value": mustParseFloat(stats["outputs"])},
        "stats_bytes":                map[string]interface{}{"value": mustParseFloat(stats["bytes"])},
        "stats_tip_height":           map[string]interface{}{"value": mustParseFloat(stats["tip_height"])},
        "stats_tip_bits":             map[string]interface{}{"value": mustParseFloat(stats["tip_bits"])},

        // /smpvStats (mempool + script types)
        "smpv_enabled":           map[string]interface{}{"value": mustParseFloat(stats["enabled"])},
        "smpv_mempool_txs":       map[string]interface{}{"value": mustParseFloat(stats["mempool_txs"])},
        "smpv_watchers":          map[string]interface{}{"value": mustParseFloat(stats["watchers"])},
        "smpv_confirmed":         map[string]interface{}{"value": mustParseFloat(stats["confirmed"])},
        "smpv_unconfirmed":       map[string]interface{}{"value": mustParseFloat(stats["unconfirmed"])},
        "smpv_total_bytes":       map[string]interface{}{"value": mustParseFloat(stats["total_bytes"])},
        "smpv_last_seen_age_sec": map[string]interface{}{"value": mustParseFloat(stats["last_seen_age_sec"])},

        "smpv_types_p2pk":        map[string]interface{}{"value": mustParseFloat(stats["types_p2pk"])},
        "smpv_types_p2pkh":       map[string]interface{}{"value": mustParseFloat(stats["types_p2pkh"])},
        "smpv_types_p2sh":        map[string]interface{}{"value": mustParseFloat(stats["types_p2sh"])},
        "smpv_types_multisig":    map[string]interface{}{"value": mustParseFloat(stats["types_multisig"])},
        "smpv_types_op_return":   map[string]interface{}{"value": mustParseFloat(stats["types_op_return"])},
        "smpv_types_nonstandard": map[string]interface{}{"value": mustParseFloat(stats["types_nonstandard"])},
        "smpv_types_vout_total":  map[string]interface{}{"value": mustParseFloat(stats["types_vout_total"])},
        "smpv_coinbase_txs":      map[string]interface{}{"value": mustParseFloat(stats["coinbase_txs"])},

        // /chainStats (session totals)
        "headers_bytes":      map[string]interface{}{"value": chain["headers_bytes"]},
        "blocks_total":       map[string]interface{}{"value": chain["blocks_total"]},
        "transactions_total": map[string]interface{}{"value": chain["transactions_total"]},
        "outputs_total":      map[string]interface{}{"value": chain["outputs_total"]},
        "output_value_total": map[string]interface{}{"value": chain["output_value_total"]},
        "fees_total":         map[string]interface{}{"value": chain["fees_total"]},
        "block_bytes_total":  map[string]interface{}{"value": chain["block_bytes_total"]},
        "approx_chain_bytes": map[string]interface{}{"value": chain["approx_chain_bytes"]},
        "chain_tip_height":   map[string]interface{}{"value": mustParseFloat(chain["tip_height"])},
        "chain_tip_bits":     map[string]interface{}{"value": mustParseFloat(chain["tip_bits"])},
        "chain_tip_time":     map[string]interface{}{"value": chain["tip_time"]},
    }

    marshalled, err := json.Marshal(payload)
    if err != nil {
        log.Printf("Error marshalling metrics: %v", err)
        return
    }

    url := fmt.Sprintf("http://%s:%s/dbx/metrics", os.Getenv("DBX_HOST"), os.Getenv("DBX_PORT"))
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(marshalled))
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
    }
}

func mustParseFloat(s string) float64 {
    f, ok := tryParseFloat(s)
    if !ok {
        log.Printf("Failed to parse float: %s", s)
        return 0
    }
    return f
}

func tryParseFloat(s string) (float64, bool) {
    f, err := strconv.ParseFloat(s, 64)
    return f, err == nil
}

func main() {
    log.Println("Sleeping to give spvnode time to start...")
    time.Sleep(10 * time.Second)

    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        m, stats, chain, err := collectMetrics()
        if err != nil {
            log.Printf("Error collecting metrics: %v", err)
            continue
        }
        log.Printf("Metrics: %+v | stats=%v | chain=%v", m, stats, chain)
        submitMetrics(m, stats, chain)
        log.Printf("----------------------------------------")
    }
}
