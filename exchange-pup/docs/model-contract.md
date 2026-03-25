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
