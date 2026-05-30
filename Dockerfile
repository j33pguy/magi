FROM golang:1.25-bookworm AS builder

# TARGETARCH is provided automatically by BuildKit (e.g. amd64, arm64).
ARG TARGETARCH
ARG ONNX_VERSION=1.26.0

RUN apt-get update && apt-get install -y gcc libc6-dev curl && rm -rf /var/lib/apt/lists/*

# Download the ONNX Runtime build matching the target architecture. ONNX Runtime
# uses "x64"/"aarch64" naming, so map from Docker's TARGETARCH accordingly.
RUN set -eux; \
    case "$TARGETARCH" in \
        amd64) ONNX_ARCH=x64 ;; \
        arm64) ONNX_ARCH=aarch64 ;; \
        *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-linux-${ONNX_ARCH}-${ONNX_VERSION}.tgz" -o /tmp/onnxruntime.tgz; \
    tar -xzf /tmp/onnxruntime.tgz -C /tmp; \
    cp "/tmp/onnxruntime-linux-${ONNX_ARCH}-${ONNX_VERSION}/lib/libonnxruntime.so" /usr/local/lib/; \
    ldconfig; \
    rm -rf /tmp/onnxruntime*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 go build -o /usr/local/bin/magi .

FROM debian:bookworm-slim

ARG VERSION
ARG REVISION
ARG BUILD_DATE

LABEL org.opencontainers.image.title="magi" \
      org.opencontainers.image.description="Multi-Agent Graph Intelligence (MAGI) memory server" \
      org.opencontainers.image.url="https://github.com/j33pguy/magi" \
      org.opencontainers.image.source="https://github.com/j33pguy/magi" \
      org.opencontainers.image.licenses="Elastic-2.0" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$REVISION \
      org.opencontainers.image.created=$BUILD_DATE

RUN apt-get update && apt-get install -y ca-certificates curl && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/lib/libonnxruntime.so /usr/local/lib/
RUN ldconfig

COPY --from=builder /usr/local/bin/magi /usr/local/bin/magi

RUN useradd -r -s /bin/false magi && \
    mkdir -p /data/models && \
    chown -R magi:magi /data
USER magi

ENV MEMORY_BACKEND=sqlite
# The sqlite backend reads SQLITE_PATH; MAGI_REPLICA_PATH is only used by the
# turso backend. Set both so /data is used regardless of MEMORY_BACKEND.
ENV SQLITE_PATH=/data/memory.db
ENV MAGI_REPLICA_PATH=/data/memory.db
ENV MAGI_MODEL_DIR=/data/models
ENV MAGI_GRPC_PORT=8300
ENV MAGI_HTTP_PORT=8301
ENV MAGI_LEGACY_HTTP_PORT=8302
ENV MAGI_UI_PORT=8080

EXPOSE 8300 8301 8302 8080

VOLUME ["/data"]

ENTRYPOINT ["magi"]
CMD ["--http-only"]
