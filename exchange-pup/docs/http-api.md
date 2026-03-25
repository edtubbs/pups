# http api

## endpoints
- `GET /health`
- `GET /status`
- `GET /config/effective`
- `GET /signals/latest`
- `GET /signals/history?limit=50`
- `GET /positions`
- `GET /inventory`
- `GET /paper/performance`
- `POST /predict`
- `POST /approve/{signal_id}`
- `POST /execute/{signal_id}`
- `POST /kill-switch/on`
- `POST /kill-switch/off`

## examples
```bash
curl -s http://127.0.0.1:8099/health
curl -s -X POST http://127.0.0.1:8099/predict
curl -s http://127.0.0.1:8099/signals/latest
```
