# Embeddings Testing

The ONNX provider (`onnx.go`) requires CGO and the ONNX Runtime shared library.
Unit tests use the mock provider which covers the `Provider` interface at 100%.

ONNX-specific functions are tested in integration tests with `--tags=integration`:

```bash
go test --tags=integration ./internal/embeddings/
```
