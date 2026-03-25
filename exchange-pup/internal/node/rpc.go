package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/edtubbs/pups/exchange-pup/internal/config"
	"github.com/edtubbs/pups/exchange-pup/internal/types"
)

type Client struct {
	httpClient *http.Client
	url        string
	user       string
	pass       string
}

func NewClient(cfg config.NodeRPCConfig) *Client {
	t := cfg.Timeout
	if t <= 0 {
		t = 5 * time.Second
	}
	return &Client{httpClient: &http.Client{Timeout: t}, url: cfg.URL, user: cfg.User, pass: cfg.Password}
}

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResp struct {
	Result json.RawMessage `json:"result"`
	Error  any             `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	body, _ := json.Marshal(rpcReq{JSONRPC: "1.0", ID: "exchange-pup", Method: method, Params: params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("rpc status %d", resp.StatusCode)
	}
	var r rpcResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if r.Error != nil {
		return fmt.Errorf("rpc error: %v", r.Error)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

func (c *Client) Snapshot(ctx context.Context) (types.NodeSnapshot, error) {
	now := time.Now().UTC()
	out := types.NodeSnapshot{Timestamp: now}

	var chainInfo struct {
		Chain   string `json:"chain"`
		Blocks  int64  `json:"blocks"`
		Headers int64  `json:"headers"`
	}
	if err := c.call(ctx, "getblockchaininfo", []any{}, &chainInfo); err != nil {
		return out, err
	}
	out.Chain = chainInfo.Chain
	out.Blocks = chainInfo.Blocks
	out.Headers = chainInfo.Headers

	var mempool struct {
		Bytes int64 `json:"bytes"`
		Size  int64 `json:"size"`
	}
	_ = c.call(ctx, "getmempoolinfo", []any{}, &mempool)
	out.MempoolBytes = mempool.Bytes
	out.MempoolTxCount = mempool.Size

	var bal float64
	_ = c.call(ctx, "getbalance", []any{}, &bal)
	out.WalletBalance = bal

	var txs []map[string]any
	_ = c.call(ctx, "listtransactions", []any{"*", 20}, &txs)
	out.RecentWalletTxCount = len(txs)

	var recent [11]map[string]any
	_ = c.call(ctx, "getblockstats", []any{chainInfo.Blocks, []string{"mediantime"}}, &recent)
	out.RecentBlockIntervalSec = 60

	return out, nil
}

func (c *Client) ExecuteSendToAddress(ctx context.Context, address string, amount float64, comment string) (string, error) {
	var txid string
	err := c.call(ctx, "sendtoaddress", []any{address, amount, comment}, &txid)
	return txid, err
}
