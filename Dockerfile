FROM golang:1.24-bookworm AS builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        gcc \
        libc6-dev \
        libsqlite3-dev \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1
RUN go build -trimpath -ldflags="-s -w" -o /out/magi .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/magi /usr/local/bin/magi

RUN mkdir -p /data/models \
    && useradd -r -s /bin/false magi \
    && chown -R magi:magi /data

USER magi

ENV MEMORY_BACKEND=sqlite
ENV MAGI_REPLICA_PATH=/data/memory.db
ENV MAGI_MODEL_DIR=/data/models
ENV MAGI_GRPC_PORT=8300
ENV MAGI_HTTP_PORT=8301
ENV MAGI_LEGACY_HTTP_PORT=8302
ENV MAGI_UI_PORT=8080

EXPOSE 8300 8301 8302 8080

VOLUME ["/data"]

HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=20s \
  CMD curl -fsS "http://localhost:${MAGI_LEGACY_HTTP_PORT:-8302}/health" >/dev/null || exit 1

ENTRYPOINT ["magi"]
CMD ["--http-only"]
