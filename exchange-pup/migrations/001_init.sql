CREATE TABLE IF NOT EXISTS market_candles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  exchange TEXT NOT NULL,
  symbol TEXT NOT NULL,
  interval TEXT NOT NULL,
  open REAL NOT NULL,
  high REAL NOT NULL,
  low REAL NOT NULL,
  close REAL NOT NULL,
  volume REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS market_ticks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  exchange TEXT NOT NULL,
  symbol TEXT NOT NULL,
  price REAL NOT NULL,
  bid REAL NOT NULL,
  ask REAL NOT NULL,
  volume REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS market_books (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  exchange TEXT NOT NULL,
  symbol TEXT NOT NULL,
  depth_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS feature_rows (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  symbol TEXT NOT NULL,
  feature_schema_version TEXT NOT NULL,
  values_json TEXT NOT NULL,
  context_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  model_version TEXT NOT NULL,
  feature_schema_version TEXT NOT NULL,
  backend TEXT NOT NULL,
  status TEXT NOT NULL,
  details_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS signals (
  id TEXT PRIMARY KEY,
  ts INTEGER NOT NULL,
  action TEXT NOT NULL,
  confidence REAL NOT NULL,
  expected_edge REAL NOT NULL,
  horizon TEXT NOT NULL,
  ranked_actions_json TEXT NOT NULL,
  reason_codes_json TEXT NOT NULL,
  reason_summary TEXT NOT NULL,
  model_version TEXT NOT NULL,
  feature_schema_version TEXT NOT NULL,
  policy_allowed INTEGER NOT NULL,
  policy_reasons_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_decisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  signal_id TEXT NOT NULL,
  allowed INTEGER NOT NULL,
  mode TEXT NOT NULL,
  requires_approval INTEGER NOT NULL,
  dry_run INTEGER NOT NULL,
  reason_codes_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS executions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  signal_id TEXT NOT NULL,
  executor TEXT NOT NULL,
  status TEXT NOT NULL,
  request_json TEXT NOT NULL,
  response_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS paper_positions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  symbol TEXT NOT NULL,
  quantity REAL NOT NULL,
  avg_price REAL NOT NULL,
  cash REAL NOT NULL,
  unrealized_pnl REAL NOT NULL,
  realized_pnl REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS paper_fills (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  signal_id TEXT NOT NULL,
  symbol TEXT NOT NULL,
  side TEXT NOT NULL,
  quantity REAL NOT NULL,
  fill_price REAL NOT NULL,
  fee REAL NOT NULL,
  slippage REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS approvals (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  signal_id TEXT NOT NULL,
  approver TEXT NOT NULL,
  approved INTEGER NOT NULL,
  notes TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS config_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  config_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_market_candles_ts_ex_symbol ON market_candles(ts, exchange, symbol);
CREATE INDEX IF NOT EXISTS idx_market_ticks_ts_ex_symbol ON market_ticks(ts, exchange, symbol);
CREATE INDEX IF NOT EXISTS idx_market_books_ts_ex_symbol ON market_books(ts, exchange, symbol);
CREATE INDEX IF NOT EXISTS idx_feature_rows_ts_symbol_schema ON feature_rows(ts, symbol, feature_schema_version);
CREATE INDEX IF NOT EXISTS idx_model_runs_ts_model_ver ON model_runs(ts, model_version);
CREATE INDEX IF NOT EXISTS idx_signals_ts_action_model ON signals(ts, action, model_version);
CREATE INDEX IF NOT EXISTS idx_policy_decisions_ts_signal ON policy_decisions(ts, signal_id);
CREATE INDEX IF NOT EXISTS idx_executions_ts_signal ON executions(ts, signal_id);
CREATE INDEX IF NOT EXISTS idx_paper_positions_ts_symbol ON paper_positions(ts, symbol);
CREATE INDEX IF NOT EXISTS idx_paper_fills_ts_signal ON paper_fills(ts, signal_id);
CREATE INDEX IF NOT EXISTS idx_approvals_ts_signal ON approvals(ts, signal_id);
