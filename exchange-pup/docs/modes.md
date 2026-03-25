# operation modes

- `recommend_only` (default): recommendations only, execution blocked by policy.
- `paper_trade`: simulated execution and ledger updates.
- `live_chain_execute`: on-chain execution through Core RPC (requires explicit flags).
- `live_exchange_execute`: scaffolded only; live order placement disabled by default.
