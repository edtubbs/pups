CREATE TABLE IF NOT EXISTS identities (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  public_key TEXT NOT NULL,
  metadata_json TEXT NOT NULL,
  trust_tier TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS attestations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id TEXT NOT NULL,
  evidence_json TEXT NOT NULL,
  trust_state TEXT NOT NULL,
  reasons_json TEXT NOT NULL,
  digest TEXT NOT NULL,
  captured_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS trust_decisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  subject_id TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  decision TEXT NOT NULL,
  reasons_json TEXT NOT NULL,
  timestamp INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS manifests (
  manifest_digest TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL,
  version TEXT NOT NULL,
  manifest_json TEXT NOT NULL,
  signature TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
  artifact_digest TEXT PRIMARY KEY,
  manifest_digest TEXT NOT NULL,
  path TEXT,
  content_type TEXT,
  size_bytes INTEGER,
  verified INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS provenance_bundles (
  bundle_hash TEXT PRIMARY KEY,
  envelope_json TEXT NOT NULL,
  verified_signatures INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS entitlements (
  entitlement_id TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL,
  subject_type TEXT NOT NULL,
  subject_id TEXT NOT NULL,
  status TEXT NOT NULL,
  starts_at INTEGER NOT NULL,
  ends_at INTEGER NOT NULL,
  metadata_json TEXT NOT NULL,
  transferable INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS transfers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entitlement_id TEXT NOT NULL,
  from_subject_id TEXT NOT NULL,
  to_subject_id TEXT NOT NULL,
  timestamp INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS revocations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ref_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  reason TEXT NOT NULL,
  timestamp INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS anchors (
  anchor_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  digest TEXT NOT NULL,
  ref TEXT,
  issuer_id TEXT,
  txid TEXT,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS fetch_cache (
  digest TEXT PRIMARY KEY,
  path TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  source_ref TEXT
);

CREATE TABLE IF NOT EXISTS config_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  config_json TEXT NOT NULL,
  timestamp INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_id TEXT,
  action TEXT NOT NULL,
  object_kind TEXT,
  object_id TEXT,
  result TEXT NOT NULL,
  reasons_json TEXT,
  timestamp INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attestations_device_id ON attestations(device_id);
CREATE INDEX IF NOT EXISTS idx_attestations_timestamp ON attestations(captured_at);
CREATE INDEX IF NOT EXISTS idx_trust_decisions_subject ON trust_decisions(subject_id, subject_kind);
CREATE INDEX IF NOT EXISTS idx_artifacts_digest ON artifacts(artifact_digest);
CREATE INDEX IF NOT EXISTS idx_manifests_digest ON manifests(manifest_digest);
CREATE INDEX IF NOT EXISTS idx_entitlements_id ON entitlements(entitlement_id);
CREATE INDEX IF NOT EXISTS idx_anchors_txid ON anchors(txid);
CREATE INDEX IF NOT EXISTS idx_anchors_created ON anchors(created_at);
CREATE INDEX IF NOT EXISTS idx_attestations_trust_state ON attestations(trust_state);
