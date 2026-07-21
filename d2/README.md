# D2 Pup

This pup runs a [D2](https://github.com/dogecoinfoundation/d2) testnet node on your dogebox.

## Packaging

The D2 node is built from the `d2` package in
[dogebox-nur-packages](https://github.com/edtubbs/dogebox-nur-packages)
(`pkgs/d2`), pinned by commit in `pup.nix`. The package builds the
`d2-node` Go daemon linked against the `libd2` Rust library.

**Note:** the D2 source repository is private, so the package fetches it
over SSH. The machine building this pup needs read access to
`dogecoinfoundation/d2` (see `pkgs/d2/source.nix` in the NUR repo for
deploy-key / sandbox details). It cannot be built by public CI or the
binary cache until the repo is public.

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
| `d2d`     | The D2 node (from dogebox-nur-packages `pkgs/d2`)                  |
| `monitor` | Reports node status/metrics to the Dogebox GUI                     |
| `logger`  | Tails the node's debug log                                         |

## Interfaces

| Interface    | Port  | Description                                  |
|--------------|-------|----------------------------------------------|
| `d2-network` | 42069 | P2P network port (listens on host)           |
| `d2-rpc`     | 42070 | RPC access for dependent pups                |
| `d2-events`  | 42071 | Block/transaction event notifications        |

## Remaining work

- [ ] Pass explicit P2P/RPC/event port flags to `d2-node` once its CLI is documented
- [ ] Implement real RPC polling in `monitor/monitor.go`
- [ ] Add archival vs light-weight node profiles (config section + flags)
- [ ] Add the fortnightly reset/bootstrap-from-D1-chainstate logic
- [ ] Update `nixFileSha256` in `manifest.json` after editing `pup.nix`
