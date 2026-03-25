# model contract

Metadata file fields:
- `model_version`
- `feature_schema_version`
- `target_labels`
- `horizon`
- optional `calibration`

Schema mismatches fail safe and block execution.

Runtime interface:
- `LoadModel(path, metadata)`
- `Predict(featureVector) -> ranked actions`

Roadmap: ONNX Runtime can be added behind a feature flag with the same interface.

## roadmap: ONNX runtime support
- Add `model_backend: onnx` as an optional backend.
- Keep ONNX disabled by default and behind explicit config flag.
- Reuse the same model interface used by fake/XGBoost backends.
- Enforce feature schema compatibility and fail-safe behavior identical to current backends.
