<div align="center">
  <img src="../docs/img/dogebox-logo.png" alt="Dogebox Logo"/>
  <p>Merchant SPV Gateway</p>
</div>

> [!CAUTION]  
> This pup is experimental and not ready for production use.

## Overview

This pup provides a lightweight Dogecoin payment gateway optimized for merchant use cases. Unlike the full node implementation, this variant uses pruning to minimize disk space requirements while maintaining the ability to accept and track payments.

## Key Features

- **Pruned Node**: Requires only ~1GB disk space instead of ~300GB
- **Wallet Enabled**: Supports payment address generation and tracking
- **Payment Gateway API**: REST API for creating addresses and monitoring payments
- **Low Resource Usage**: Optimized for merchant payment processing

## Disk Requirements

Approximately **1-2GB** of disk space (pruned blockchain data)

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

### Health Check
```
GET /health
Response: {"status": "operational", "time": "..."}
```

## Services

This pup runs four services:

1. **node-daemon**: Pruned Dogecoin node with wallet enabled
2. **payment-gateway**: REST API for payment processing
3. **health-checker**: Monitors node status and reports metrics
4. **log-stream**: Streams node logs for debugging

## Configuration

Configure the gateway through the Dogebox interface:

- **Minimum Confirmations**: Number of confirmations before marking payment as final (default: 6)
- **Notification URL**: Optional webhook endpoint for payment notifications

## Security Notes

- RPC credentials are automatically generated and stored securely
- The wallet is named "payments" and uses legacy address format
- Only exposes payment-related RPC calls through the gateway API
- All internal communication stays within the Dogebox network (10.69.0.0/16)
