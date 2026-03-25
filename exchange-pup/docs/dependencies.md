# dependencies and build notes

## go dependencies
- `modernc.org/sqlite` for pure-Go SQLite runtime access.
- `gopkg.in/yaml.v3` for config parsing.
- `prometheus/client_golang` for metrics export.

## native/runtime expectations
- XGBoost C API is scaffolded as an optional backend; current default backend is `fake`.
- TA-Lib indicators are currently implemented with Go fallbacks for immediate portability.
- Future native linking for TA-Lib/XGBoost can be added without changing public interfaces.

## security defaults
- No secrets are logged; effective config redacts credentials.
- Live execution requires explicit config opt-in flags.
