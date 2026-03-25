# Threat Model

Primary threats addressed:
- unsigned or tampered artifacts
- untrusted or stale device state
- forged entitlement state
- revocation bypass
- unauthorized live on-chain writes

Design constraints:
- fail closed on missing or invalid evidence
- no implicit trust of unsigned content
- no unauthenticated execution
- trust decisions are explainable and logged
