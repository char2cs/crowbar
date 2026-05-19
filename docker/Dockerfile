FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy Go source first
COPY api/ ./api/

# Copy web dist into the location expected by go:embed (api/cmd/crowbar/web/dist).
# web/dist must be built before running docker build.
COPY web/dist ./api/cmd/crowbar/web/dist

WORKDIR /build/api

RUN CGO_ENABLED=0 go build -o /crowbar ./cmd/crowbar

FROM alpine:3.21
RUN apk add --no-cache ca-certificates

COPY --from=builder /crowbar /usr/local/bin/crowbar

EXPOSE 3737

ENTRYPOINT ["crowbar", "serve", "--host", "tcp://0.0.0.0:3737"]
