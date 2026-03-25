# exchange-pup

AI-assisted DOGE market timing and routing pup for Dogebox.

## quick start (recommend-only)
1. Copy `contrib/config.recommend-only.yaml` to `/storage/exchange-pup.yaml`.
2. Copy `contrib/model-metadata.example.json` to `/storage/model-metadata.json`.
3. Run `/bin/exchange-pup -config /storage/exchange-pup.yaml`.
4. Call:
   - `GET /health`
   - `POST /predict`
   - `GET /signals/latest`

Default backend is `fake` for immediate development/testing without a trained model.

## security defaults
- recommendation-only mode
- dry-run enabled
- live execution flags disabled
- whitelist-enforced policy checks
- redacted effective config output
