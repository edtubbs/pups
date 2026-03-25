# paper trading

Paper executor simulates fills with configured assumptions:
- `assumed_fee_bps`
- `assumed_slippage_bps`

Simulated executions are stored in:
- `executions`
- `paper_fills`
- `paper_positions`

Use `GET /paper/performance` for current assumptions and state.
