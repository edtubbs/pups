# Architecture

`digital-asset-signing-pup` is a verification-first trust and distribution control plane for Dogebox.

## Modules

- `internal/config`: strict config loading and fail-closed validation.
- `internal/storage`: SQLite metadata state and migrations.
- `internal/blockchain`: Dogecoin RPC anchor encode/create/verify with minimal payload strategy.
- `internal/identity`: provider/device/developer/operator identity registration and anchor linkage.
- `internal/attestation`: TPM-first evidence verification with explicit trust states.
- `internal/trust`: manifest signatures, DSSE/in-toto provenance checks, revocation lookups.
- `internal/distribution`: digest-first fetch and cache.
- `internal/licensing`: entitlements, transfer, revocation.
- `internal/policy`: final allow/deny gating.
- `internal/httpapi`: local operator API.
- `internal/telemetry`: metrics endpoint.

## Trust flow

1. Register identities.
2. Collect and verify attestation.
3. Verify signed metadata and provenance.
4. Fetch artifact by digest.
5. Evaluate policy.
6. Optionally anchor compact records on Dogecoin when policy allows.

All dangerous operations are explicit and off by default.
