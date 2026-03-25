# HTTP API

Read endpoints:
- GET /health
- GET /status
- GET /trust/device/{id}
- GET /trust/provider/{id}
- GET /artifacts/{id}
- GET /licenses/{id}
- GET /anchors/{id}
- GET /attestations/{device_id}/latest

Write endpoints:
- POST /device/register
- POST /device/attest
- POST /provider/register
- POST /artifact/publish
- POST /artifact/verify
- POST /artifact/fetch
- POST /license/create
- POST /license/transfer
- POST /anchor/create
- POST /policy/evaluate
- POST /kill-switch/on
- POST /kill-switch/off
