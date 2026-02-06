<div align="center">
  <img src="../docs/img/dogebox-logo.png" alt="Dogebox Logo"/>
  <p>SPV Merchant Gateway</p>
</div>

> [!CAUTION]  
> This pup is experimental and not ready for production use.

## Overview

This pup provides a lightweight Dogecoin payment gateway using **libdogecoin's spvnode** for merchant use cases. Unlike full node implementations, this uses Simple Payment Verification (SPV) to verify transactions with only block headers, minimizing disk space and sync time.

The gateway uses a **script wrapper approach** to expose libdogecoin tools (`spvnode`, `such`, `sendtx`) through a unified `gateway.sh` script, providing a clean interface for payment operations.

## What is libdogecoin?

[Libdogecoin](https://github.com/dogecoinfoundation/libdogecoin) is a clean C library implementation of Dogecoin building blocks. It provides lightweight tools including:
- `spvnode` - SPV node for blockchain sync and wallet management
- `such` - CLI tool for key and address generation
- `sendtx` - Transaction broadcasting utility

## Key Features

- **SPV Node**: Syncs only block headers (~50-100MB) instead of full blockchain (~300GB)
- **Fast Sync**: Catches up to chain tip in minutes, not hours
- **Wallet Support**: Built-in wallet database for address and transaction tracking
- **Payment Gateway API**: REST API for creating addresses and monitoring payments
- **Low Resource**: Optimized for resource-constrained environments

## Disk Requirements

Approximately **50-100MB** for block headers (vs ~300GB for full node)

## How SPV Works

SPV (Simplified Payment Verification) nodes:
1. Download only block headers (80 bytes each)
2. Verify proof-of-work chain integrity
3. Request specific transactions when needed
4. Validate transactions using Merkle proofs

This provides strong security guarantees while using minimal resources.

## API Endpoints

The merchant gateway exposes a REST API on port 8080:

### Generate New Payment Address
```
POST /api/address/new
Body: {"label": "Order #12345"}
Response: {"address": "D...", "label": "Order #12345", "created_at": "..."}
```

### List All Addresses
```
GET /api/address/list
Response: [{"address": "D...", "label": "...", "created_at": "..."}]
```

### Check Payments
```
GET /api/payments
Response: [{"txid": "...", "address": "...", "amount": 100.0, "confirmations": 3}]
```

### Broadcast Transaction
```
POST /api/transaction/broadcast
Body: {"hex": "01000000..."}
Response: {"status": "broadcasted", "output": "..."}
```

### Health Check
```
GET /health
Response: {"status": "operational", "time": "..."}
```

## Services

This pup runs four services:

1. **spvnode**: Libdogecoin SPV node (runs in continuous mode with full block sync)
2. **payment-gateway**: REST API for payment processing (uses gateway.sh wrapper script)
3. **health-checker**: Monitors node sync status and reports metrics
4. **log-stream**: Streams spvnode logs for debugging

## Gateway Script Wrapper

The `gateway.sh` script provides a unified interface to libdogecoin tools:

- **generateAddress** - Creates new payment addresses using `such` CLI
- **listAddresses** - Lists all generated addresses from storage
- **broadcastTransaction** - Broadcasts transactions using `sendtx`

This wrapper approach:
- Simplifies tool integration
- Provides consistent JSON output
- Handles key storage and management
- Enables easy command-line testing

Example usage:
```bash
/bin/gateway.sh generateAddress "Order #123"
/bin/gateway.sh listAddresses
/bin/gateway.sh broadcastTransaction <hex>
```

## Configuration

Configure the gateway through the Dogebox interface:

- **Minimum Confirmations**: Number of confirmations before marking payment as final (default: 6)
- **Notification URL**: Optional webhook endpoint for payment notifications

## SPV Node Flags

The spvnode runs with these flags:
- `-c` - Continuous mode (keeps running and waiting for new blocks)
- `-b` - Full block mode (downloads full blocks for transaction verification)
- `-p` - Checkpoint mode (uses checkpoints for faster initial sync)
- `-w` - Wallet file path
- `-h` - Headers database file path
- `-l` - No prompt mode (loads wallet/headers automatically)

## Technical Details

- **Language**: C library with Go services and shell script wrappers
- **Dependencies**: libevent, libunistring, awk, jq
- **Network**: Connects to Dogecoin P2P network on port 22556
- **Storage**: SQLite databases for headers and wallet data
- **Security**: Local key generation using libdogecoin's cryptographic functions

## Advantages of SPV

✅ **Minimal Disk Usage**: ~100MB vs ~300GB  
✅ **Fast Sync**: Minutes vs hours/days  
✅ **Low Bandwidth**: Only downloads what's needed  
✅ **Secure**: Validates block headers and Merkle proofs  
✅ **Merchant-Focused**: Designed for payment acceptance  

## Limitations

⚠️ **Privacy**: SPV nodes may reveal addresses to peers when requesting transactions  
⚠️ **Trust**: Relies on honest majority of hashpower (same as full nodes)  
⚠️ **Features**: Does not support all RPC calls available in full nodes  

## Comparison to Core Pup

| Feature | Core Pup | SPV Merchant Gateway |
|---------|----------|----------------------|
| Implementation | Dogecoin Core (C++) | libdogecoin (C) |
| Blockchain Data | Full (~300GB) | Headers only (~100MB) |
| Sync Time | Hours/Days | Minutes |
| Wallet | Optional | Built-in |
| RPC Interface | Full | None (REST API only) |
| Use Case | Full node operation | Merchant payments |

## Security Notes

- Keys are generated locally using libdogecoin's cryptographic functions
- Wallet database is stored in `/storage/merchant_wallet.db`
- Headers database is stored in `/storage/headers.db`
- All network communication is peer-to-peer encrypted
- Payment gateway API is isolated within Dogebox network (10.69.0.0/16)

## Learn More

- [Libdogecoin GitHub](https://github.com/dogecoinfoundation/libdogecoin)
- [Libdogecoin Documentation](https://github.com/dogecoinfoundation/libdogecoin/tree/main/doc)
- [SPV Whitepaper Section](https://bitcoin.org/bitcoin.pdf) (Section 8 - Simplified Payment Verification)
