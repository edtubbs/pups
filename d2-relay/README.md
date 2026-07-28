# D2 Relay Pup

This pup relays UTXO activity from Dogecoin (D1) to the
[D2 testnet](../d2), enabling stress test scenarios against a realistic
UTXO set, and reports testnet metrics to the Dogebox GUI.

## What it does

- Polls the D1 core pup's RPC interface (`core-rpc` dependency) for new blocks.
- For each new block, extracts the UTXOs sent (inputs spent) and received
  (outputs created) by every transaction.
- Relays each observed D1 transaction to the D2 node via
  `d2_sendRawTransaction`.
- Detects **testnet epoch changes** (a new `chainId` from `d2_getInfo` after
  the fortnightly reset from a fresh D1 snapshot).

## Metrics (shown in the Dogebox GUI)

| Metric             | Description                                       |
|--------------------|---------------------------------------------------|
| `d1_height`        | Latest processed D1 block height                  |
| `d2_height`        | D2 chain tip height reported by `d2_getInfo`      |
| `last_block`       | Hash of the last processed D1 block               |
| `blocks_processed` | D1 blocks scanned since the relay started         |
| `utxos_created`    | Total UTXOs created (outputs seen)                |
| `utxos_spent`      | Total UTXOs spent (inputs seen)                   |
| `relayed_txs`      | Total transactions relayed to D2                  |
| `failed_txs`       | Transactions that failed to relay into D2         |
| `doge_relayed`     | Total DOGE value of successfully relayed txs      |
| `relay_lag`        | D1 height minus D2 tip height (floored at 0)      |

## Dependencies

- `core-rpc` (v0.0.1) from the [Dogecoin Core pup](../core)
- `d2-rpc` (v0.0.1) from the [D2 pup](../d2)

## D2 RPC auth

- D2 write methods require a bearer token. Set one of:
  - `DBX_IFACE_D2_RPC_BEARER_TOKEN` (preferred)
  - `D2_RPC_BEARER_TOKEN` (fallback)
- The relay sends this token in the HTTP `Authorization` header when calling
  `d2_sendRawTransaction`.

## Remaining work

- [ ] Add config toggles for stress-test scenarios (replay rate, burst mode)
- [x] Add relay lag metric (D1 height vs D2 processed height)
