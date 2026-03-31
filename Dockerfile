FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates curl && rm -rf /var/lib/apt/lists/*

COPY magi /usr/local/bin/magi

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
