# D2 Pup

This pup runs a [D2](https://github.com/dogecoinfoundation/d2) testnet node on your dogebox.

## Status: Scaffolding

The D2 node binary does not yet have a public release artifact, so this pup is
currently **scaffolding**. The `d2d` service is a placeholder that waits until
a real D2 package is plugged into `pup.nix` (see the `TODO` there). The
manifest, service layout, interfaces, and metrics are all in place so that
enabling the real node is a small follow-up change.

## Testnet model

- The D2 testnet is **planned to reset every two weeks** from a Dogecoin (D1)
  chainstate snapshot, giving the network a realistic UTXO set for stress
  testing. The reset/bootstrap logic is not yet implemented (see remaining
  work below).
- The companion [`d2-relay`](../d2-relay) pup monitors UTXOs sent/received in
  D1 blocks and relays them to the D2 testnet, and reports testnet metrics
  (including double-spend detection) to the Dogebox GUI.

## Services

| Service   | Description                                                        |
|-----------|--------------------------------------------------------------------|
| `d2d`     | The D2 node (placeholder until a release artifact is available)    |
| `monitor` | Reports node status/metrics to the Dogebox GUI                     |
| `logger`  | Tails the node's debug log                                         |

## Interfaces

| Interface    | Port  | Description                                  |
|--------------|-------|----------------------------------------------|
| `d2-network` | 42069 | P2P network port (listens on host)           |
| `d2-rpc`     | 42070 | RPC access for dependent pups                |
| `d2-events`  | 42071 | Block/transaction event notifications        |

## Remaining work (once a D2 artifact is published)

- [ ] Replace the placeholder derivation in `pup.nix` with the real D2 package
- [ ] Launch the node in `run.sh` with the ports/datadir documented in `pup.nix`
- [ ] Implement real RPC polling in `monitor/monitor.go`
- [ ] Add archival vs light-weight node profiles (config section + flags)
- [ ] Add the fortnightly reset/bootstrap-from-D1-chainstate logic
- [ ] Update `nixFileSha256` in `manifest.json` after editing `pup.nix`
