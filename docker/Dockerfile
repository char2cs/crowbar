FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy Go source first
COPY api/ ./api/

# Copy web dist into the location expected by go:embed (api/cmd/crowbar/web/dist).
# web/dist must be built before running docker build.
COPY web/dist ./api/cmd/crowbar/web/dist

WORKDIR /build/api

# Precompressed .gz siblings, served by api/internal/api/static.go with no
# request-time compression. Same step as api/Makefile's embed-web; busybox gzip
# (alpine) accepts these flags and always writes a zero mtime.
RUN find cmd/crowbar/web/dist -type f \( -name '*.js' -o -name '*.mjs' -o -name '*.css' -o -name '*.html' -o -name '*.json' -o -name '*.svg' -o -name '*.wasm' -o -name '*.ttf' \) -size +1k -exec gzip -k -n -9 {} +

RUN CGO_ENABLED=0 go build -o /crowbar ./cmd/crowbar

FROM alpine:3.21
RUN apk add --no-cache ca-certificates

COPY --from=builder /crowbar /usr/local/bin/crowbar

EXPOSE 3737

ENTRYPOINT ["crowbar", "serve", "--host", "tcp://0.0.0.0:3737"]
