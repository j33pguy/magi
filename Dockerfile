FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y gcc libc6-dev && rm -rf /var/lib/apt/lists/*

ADD https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-linux-x64-1.22.0.tgz /tmp/onnxruntime.tgz
RUN tar -xzf /tmp/onnxruntime.tgz -C /tmp && \
    cp /tmp/onnxruntime-linux-x64-1.22.0/lib/libonnxruntime.so /usr/local/lib/ && \
    ldconfig && \
    rm -rf /tmp/onnxruntime*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 go build -o /usr/local/bin/magi .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates curl && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/lib/libonnxruntime.so /usr/local/lib/
RUN ldconfig

COPY --from=builder /usr/local/bin/magi /usr/local/bin/magi

RUN mkdir -p /data/models && useradd -r -s /bin/false magi
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

ENTRYPOINT ["magi"]
CMD ["--http-only"]
