<div align="center">
  <img src="../docs/img/dogebox-logo.png" alt="Dogebox Logo"/>
  <p>Dogecoin Core</p>
</div>

> [!CAUTION]  
> This pup does not have a stable release yet.

This pup will install [Dogecoin Core](https://github.com/dogecoin/dogecoin) as a pup on your node.

It will install with a disabled wallet (at compile time) and no UI.

It will also start automatically syncing the blockchain, meaning you may require `~300gb` of free disk space.

## D2 chainstate snapshots

This pup builds Dogecoin Core from the `dumptxoutset`/`loadtxoutset` backport
branch and runs a `snapshot` service that exports the D1 chainstate for the
[D2 testnet](../d2) handoff. On the fortnightly reset schedule (or on demand
via `POST /snapshot/refresh`) it records `getbestblockhash`, calls the
`dumptxoutset` RPC, verifies the dump's base block and structure (stripping
any bytes after the last coin record), and publishes `utxo.dat` plus a
metadata document (base blockhash, height, coins count, file sha256) over
the `core-snapshot` HTTP interface (port 28555):

- `GET /snapshot/metadata.json`
- `GET /snapshot/utxo.dat`
- `POST /snapshot/refresh`
