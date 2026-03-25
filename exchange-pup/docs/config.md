# config reference

Primary file: `contrib/config.recommend-only.yaml`

Key safety defaults:
- `mode: recommend_only`
- `enable_live_chain_execution: false`
- `enable_live_exchange_execution: false`
- `dry_run: true`
- `recommend_only_master_switch: true`

Secrets should be provided via environment variables:
- `CORE_RPC_USER`
- `CORE_RPC_PASSWORD`
- `BINANCE_API_KEY`
- `BINANCE_API_SECRET`
- `KRAKEN_API_KEY`
- `KRAKEN_API_SECRET`

`GET /config/effective` returns redacted secrets.
