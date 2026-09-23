FROM node:22.23.2-bookworm-slim AS web-builder
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@10.17.1 --activate
COPY web/package.json web/pnpm-lock.yaml ./web/
RUN pnpm --dir web install --frozen-lockfile
COPY web ./web
RUN pnpm --dir web test
RUN pnpm --dir web build

FROM golang:1.25.14-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go test ./...
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM mwader/static-ffmpeg:7.1.1 AS media-tools

FROM debian:12.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 10001 clipweaver \
    && useradd -u 10001 -g clipweaver -d /app clipweaver \
    && mkdir -p /app/data /app/web/dist \
    && chown -R clipweaver:clipweaver /app/data
WORKDIR /app
COPY --from=media-tools /ffmpeg /ffprobe /usr/local/bin/
COPY --from=go-builder /out/server /app/server
COPY --from=web-builder /src/web/dist /app/web/dist
ENV APP_ADDR=:8080 DATA_DIR=/app/data WEB_DIST_DIR=/app/web/dist
USER clipweaver
EXPOSE 8080
CMD ["/app/server"]
