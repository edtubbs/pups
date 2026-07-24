# D2 Pup

This pup runs a [D2](https://github.com/dogecoinfoundation/d2) testnet node on your dogebox.

## Packaging

The D2 node is built from the `k2` package in
[dogebox-nur-packages](https://github.com/edtubbs/dogebox-nur-packages)
(`pkgs/k2`), pinned by commit in `pup.nix`.

**Note:** the K2 source repository is private, so the package fetches it
over SSH. The machine building this pup needs read access to
`houseofdoge/km2` (see `pkgs/k2/source.nix` in the NUR repo for sandbox
details). It cannot be built by public CI or the binary cache until the
repo is public.

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
| `d2d`     | The D2 node, run on testnet (`D2_NETWORK=testnet`), data in `/storage`, P2P on 42069, JSON-RPC 2.0 on 42070 |
| `monitor` | Polls the node's public read-tier RPC (`d2_getInfo`, `d2_getHealth`, `d2_getValidatorSet`) and reports status/metrics to the Dogebox GUI |
| `logger`  | Tails the node's debug log                                         |

## Interfaces

| Interface    | Port  | Description                                  |
|--------------|-------|----------------------------------------------|
| `d2-network` | 42069 | P2P network port (listens on host)           |
| `d2-rpc`     | 42070 | RPC access for dependent pups                |
| `d2-events`  | 42071 | Block/transaction event notifications        |

## Remaining work

- [ ] Package the actual `d2-node` Go daemon in dogebox-nur-packages (the
      current `pkgs/k2` builds the km2 key-management **library**, which
      installs no binaries)
- [ ] Wire the write-tier RPC bearer token (`/storage/rpc.token`) to dependent
      pups (d2-relay) via the `d2-rpc` interface
- [ ] Add archival vs light-weight node profiles (config section + `Role`)
- [ ] Add the fortnightly reset/bootstrap-from-D1-chainstate logic
