# D2 Pup

This pup runs a [D2](https://github.com/dogecoinfoundation/d2) testnet node on your dogebox.

## Packaging

The D2 node daemon (`d2-node`) is built from the `d2` package in
[dogebox-nur-packages](https://github.com/edtubbs/dogebox-nur-packages)
(`pkgs/d2`), pinned by commit in `pup.nix`.

**Note:** the d2 source repository is private, so the package fetches it
over SSH. The machine building this pup needs read access to
`dogecoinfoundation/d2` (see `pkgs/d2/source.nix` in the NUR repo for
sandbox details). It cannot be built by public CI or the binary cache
until the repo is public.

## Testnet model

- The D2 testnet **resets every two weeks** from a Dogecoin (D1) chainstate
  snapshot, giving the network a realistic UTXO set for stress testing.
- The core pup's snapshot service (`core-snapshot` dependency) publishes a
  raw `dumptxoutset` v1 file (`utxo.dat`) plus metadata (base blockhash,
  height, coins count, file sha256). At startup, if a new snapshot epoch
  (different base blockhash) is available, `run.sh` downloads the file,
  verifies its sha256 against the metadata, wipes the previous chain data
  (preserving `rpc.token` and logs), and starts the node with
  `--network testnet --d1-snapshot /storage/utxo.dat` so the real testnet
  chain engine is bootstrapped from the dump. (D2 has only mainnet, testnet
  and regtest networks — there is no "devnet".) Without a snapshot the node
  falls back to a plain testnet start.
- The handoff is **genesis-time only and single-shot**: there is no runtime
  `loadtxoutset`-style import, and a failed import simply aborts node start
  (cheap to retry after replacing the file). Chain correctness (right
  chain/right block) is verified on the core side before the dump is
  published — d2 itself only validates magic + version + structural
  decoding.
- **Memory budget:** the whole snapshot file is read into memory and handed
  to libd2 as one byte slice, and parsing builds the full entry vector plus
  sorted Merkle trees in RAM — budget several times the file size. The
  mainnet snapshot import (203M UTXOs) currently peaks **>12G in pass 2**;
  use a testnet-sized snapshot until libd2's import memory fix lands, and
  give the `d2d` service a generous memory allowance.
- **Restarts:** genesis is rebuilt from the snapshot on **every restart**
  (no persistence yet), so avoid restart loops on slow imports — raise the
  systemd start timeout for `d2d.service` (or disable
  restart-on-startup-timeout) so a long import is not killed and re-run
  from scratch.
- The node runs with debug-level logging; stdout/stderr are captured to
  `/storage/debug.log` (tailed by the `logger` service), showing the
  snapshot header, import pass progress with rate/ETA, "chain engine
  started", "chain genesis ready", "node started", per-RPC "rpc call"
  lines, "block produced" and the 60s "node status" heartbeat.
- After the genesis import, the node keeps **following the D1 chain** via
  its `d1follow` module: the node is started with `--d1-follow-dir
  /storage/d1follow` and polls that directory for files named
  `<height>.blk` containing canonical raw D1 block bytes (exactly what
  Core's `getblock <hash> 0` returns). Each ingested block updates the
  migration index (Tier-B candidates, spent-on-D1 markers, resume
  checkpoint) and the file is deleted once durably applied. The
  `d1follower` service in this pup writes those files: it polls the core
  pup's RPC (`core-rpc` dependency) for confirmed D1 blocks (12
  confirmations, reorg safety), stages each block on the same filesystem
  and atomically renames it into the follow dir, applies backpressure when
  the node falls behind, and resets on epoch rollover (new snapshot base
  hash). The node's ingest progress is scraped from its Prometheus
  `/metrics` listener (`d2_d1_height`, `d2_d1_blocks_total`,
  `d2_d1_migration_candidates`, `d2_d1_utxo_spent_total`) and reported to
  the Dogebox GUI. This replaces the retired `d2-relay` pup, which
  forwarded raw D1 tx hex to `d2_sendRawTransaction` — an RPC that accepts
  only canonical D2 transaction bytes, so every call failed by design.
- The node also **relays unconfirmed D1 transactions**: raw D1 transactions
  are wrapped in a new `D1Relay` D2 transaction type, gossiped through the
  D2 mempool and included in D2 blocks. The `d1mempool` service feeds that
  relay: it polls the core pup's mempool (`getrawmempool`), fetches each new
  transaction's canonical raw bytes (`getrawtransaction <txid> 0`) and
  submits them to the node's authenticated RPC tier (bearer token from
  `/storage/rpc.token`). Each transaction is submitted once, txids that
  leave the D1 mempool (mined or evicted) are forgotten so the tracking set
  stays bounded, and submissions are capped per poll so a large backlog
  cannot flood the node. Confirmed blocks keep flowing separately through
  the `d1follow` file drop above.
- The relay RPC method name is auto-detected on first use from a small
  candidate list (`d2_sendRawD1Transaction`, `d2_relayD1Transaction`,
  `d2_sendRawD1Tx`, `d2_submitD1Transaction`); a "method not found" reply
  moves on to the next candidate. Set `D1MEMPOOL_RPC_METHOD` to pin the
  method explicitly. If no candidate exists on the pinned node build the
  service just reports metrics — it never affects node startup or the
  confirmed-block path.

## Services

| Service   | Description                                                        |
|-----------|--------------------------------------------------------------------|
| `d2d`     | The D2 node, data in `/storage`, P2P on 42069, JSON-RPC 2.0 on 42070; boots from the D1 snapshot (`--network testnet --d1-snapshot`) when one is available, plain testnet otherwise |
| `monitor` | Polls the node's public read-tier RPC (`d2_getInfo`, `d2_getHealth`, `d2_getValidatorSet`) and reports status/metrics to the Dogebox GUI |
| `logger`  | Tails the node's debug log                                         |
| `d1follower` | Polls core RPC for confirmed D1 blocks and atomically drops their canonical raw bytes as `<height>.blk` files into `/storage/d1follow` for the node's `d1follow` module; reports follower metrics to the Dogebox GUI |
| `d1mempool` | Polls core RPC for unconfirmed D1 transactions and submits their canonical raw bytes to the node's D1 transaction relay, which wraps them as `D1Relay` D2 transactions; reports relay metrics to the Dogebox GUI |

## Metrics (shown in the Dogebox GUI)

| Metric          | Source RPC            | Description                                  |
|-----------------|-----------------------|----------------------------------------------|
| `status`        | `d2_getInfo`/`d2_getHealth` | Running / Syncing / degraded status    |
| `chain`         | `d2_getInfo`          | Network name (`d2-testnet`)                  |
| `blocks`        | `d2_getInfo`          | Finalized block height                       |
| `headers`       | `d2_getInfo`          | Known chain tip height                       |
| `testnet_epoch` | `d2_getValidatorSet`  | Current validator epoch                      |
| `peers`         | `d2_getInfo`          | Connected P2P peers                          |
| `mempool_txs`   | `d2_getMempool`*      | Transactions in the local mempool            |
| `finality_lag`  | `d2_getHealth`        | Blocks between tip and finalized head        |
| `last_block`    | `d2_getFinalizedHead` | Hash of the last finalized block             |
| `validators`    | `d2_getValidatorSet`  | Validators in the current epoch              |
| `d1_tip`        | core `getblockcount`  | Current D1 chain tip height                  |
| `d1_height`     | node `/metrics` (`d2_d1_height`) | d1follow ingest checkpoint        |
| `d1_blocks_total` | node `/metrics` (`d2_d1_blocks_total`) | D1 blocks ingested by the node |
| `d1_blocks_written` | d1follower      | Block files dropped into the follow dir      |
| `d1_pending`    | follow dir            | Block files awaiting ingestion               |
| `d1_follow_lag` | derived               | Confirmed D1 height minus ingest checkpoint  |
| `d1_migration_candidates` | node `/metrics` | Tier-B migration candidates          |
| `d1_utxo_spent_total` | node `/metrics` | Spent-on-D1 markers in the migration index |
| `d1_mempool_txs` | core `getrawmempool` | Unconfirmed D1 transactions available for relay |
| `d1_relayed_total` | d1mempool      | D1 transactions relayed into the D2 mempool  |
| `d1_relay_pending` | d1mempool     | D1 mempool transactions not yet relayed      |
| `d1_relay_failed` | d1mempool      | Relay attempts rejected by the D2 node       |

\* `d2_getMempool` is on the authenticated RPC tier; the monitor reads the
bearer token from `/storage/rpc.token` (written by the node on first start)
and skips the metric gracefully if unavailable.

## Interfaces

| Interface    | Port  | Description                                  |
|--------------|-------|----------------------------------------------|
| `d2-network` | 42069 | P2P network port (listens on host)           |
| `d2-rpc`     | 42070 | RPC access for dependent pups                |
| `d2-events`  | 42071 | Block/transaction event notifications        |

## Dependencies

- `core-snapshot` (v0.0.1) from the [Dogecoin Core pup](../core) — D1 UTXO
  chainstate snapshots for the fortnightly testnet reset
- `core-rpc` (v0.0.1) from the [Dogecoin Core pup](../core) — confirmed D1
  blocks for the `d1follower` service and unconfirmed D1 mempool
  transactions for the `d1mempool` service

## Remaining work

- [x] Package the actual `d2-node` Rust daemon in dogebox-nur-packages
      (`pkgs/d2`, installs `bin/d2-node`)
- [x] Add the fortnightly reset/bootstrap-from-D1-chainstate logic
- [x] Feed the node canonical raw D1 blocks via `--d1-follow-dir`
      (`d1follower` service; replaces the retired `d2-relay` pup)
- [x] Feed the node unconfirmed D1 transactions for the `D1Relay` relay
      (`d1mempool` service)
- [ ] Add archival vs light-weight node profiles (config section + `Role`)
