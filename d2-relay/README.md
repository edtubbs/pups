# D2 Relay Pup

This pup relays UTXO activity from Dogecoin (D1) to the
[D2 testnet](../d2), enabling stress test scenarios against a realistic
UTXO set, and reports testnet metrics to the Dogebox GUI.

## What it does

- Polls the D1 core pup's RPC interface (`core-rpc` dependency) for new blocks.
- For each new block, extracts the UTXOs sent (inputs spent) and received
  (outputs created) by every transaction.
- Detects **double spends** — conflicting spends of the same outpoint — and
  counts them as a testnet metric.
- Relays each transaction's UTXO activity to the D2 testnet. **The D2
  submission is currently a stub** (see `relayToD2` in `relay/relay.go`) —
  the [D2 pup](../d2) now runs a real node (built from
  dogebox-nur-packages `pkgs/d2`), but its RPC interface is not yet
  documented; the D1 monitoring side is fully functional.

## Metrics (shown in the Dogebox GUI)

| Metric          | Description                                    |
|-----------------|------------------------------------------------|
| `d1_height`     | Latest processed D1 block height               |
| `last_block`    | Hash of the last processed D1 block            |
| `utxos_created` | Total UTXOs created (outputs seen)             |
| `utxos_spent`   | Total UTXOs spent (inputs seen)                |
| `relayed_txs`   | Total transactions relayed to D2               |
| `double_spends` | Conflicting spends of the same outpoint seen   |

## Dependencies

- `core-rpc` (v0.0.1) from the [Dogecoin Core pup](../core)

## Remaining work (once the D2 RPC interface is documented)

- [ ] Add a `d2-rpc` dependency to `manifest.json`
- [ ] Implement `relayToD2` against the D2 node's RPC interface
- [ ] Add config toggles for stress-test scenarios (replay rate, burst mode)
- [ ] Add relay lag metric (D1 height vs D2 processed height)
