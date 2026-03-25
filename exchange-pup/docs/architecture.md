# exchange-pup architecture

exchange-pup is a self-contained, optional pup for DOGE market timing and routing.

## core flow
1. Ingest market snapshots from Binance/Kraken adapters.
2. Ingest local node/wallet data from Dogecoin Core RPC.
3. Build feature vectors with schema versioning.
4. Run local inference through a model backend (`fake` default, `xgboost` scaffold).
5. Rank actions and attach reason codes.
6. Apply hard policy checks.
7. Persist all artifacts to SQLite.
8. Expose API + metrics.

## module boundaries
- `internal/model`: inference only.
- `internal/policy`: final execution authority.
- `internal/execution`: executors only.
- `internal/node`: RPC transport/data pulls.
- `internal/market`: exchange adapters/normalization.

Policy and model logic are intentionally separated.
