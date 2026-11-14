# WEB selects the SPA baked into the binary: "embed" (default) needs a prebuilt
# web/dist; "none" builds with -tags noEmbed for a headless API-only daemon.
ARG WEB=embed

# Both stages expose /dist so the builder's COPY is the same either way; the
# unselected stage is never built, so "none" does not need web/dist.
FROM alpine:3.21 AS web-embed
COPY web/dist /dist

FROM alpine:3.21 AS web-none
RUN mkdir /dist

FROM web-${WEB} AS web

# Must satisfy api/go.mod's `go` directive.
FROM golang:1.26-alpine AS builder
ARG WEB

WORKDIR /build/api
COPY api/go.mod api/go.sum ./
RUN go mod download

COPY api/ ./
# go:embed reads api/cmd/crowbar/web/dist; with noEmbed it is an unused empty dir.
COPY --from=web /dist ./cmd/crowbar/web/dist

# Precompressed .gz siblings (api/internal/api/static.go serves them with no
# request-time compression), as api/Makefile's embed-web does; busybox gzip writes
# a zero mtime. A headless build has nothing to compress.
RUN [ "$WEB" = none ] || find cmd/crowbar/web/dist -type f \( -name '*.js' -o -name '*.mjs' -o -name '*.css' -o -name '*.html' -o -name '*.json' -o -name '*.svg' -o -name '*.wasm' -o -name '*.ttf' \) -size +1k -exec gzip -k -n -9 {} +

RUN TAGS=""; [ "$WEB" = none ] && TAGS=noEmbed; \
    CGO_ENABLED=0 go build -tags "$TAGS" -o /crowbar ./cmd/crowbar

FROM alpine:3.21
# git and openssh-client: the daemon creates worktrees and clones over ssh.
# tini: PID 1 must reap the processes the daemon spawns.
RUN apk add --no-cache ca-certificates git openssh-client tini \
    && adduser -D -h /home/crowbar -s /bin/sh crowbar \
    && install -d -o crowbar -g crowbar -m 700 /home/crowbar/.crowbar \
    && install -d -o crowbar -g crowbar -m 755 /home/crowbar/projects

COPY --from=builder /crowbar /usr/local/bin/crowbar

USER crowbar
# ~/.gitconfig is outside every volume, so `git config --global` is pointed at a
# file inside the CROWBAR_HOME volume to keep the commit identity across recreates.
ENV HOME=/home/crowbar \
    CROWBAR_HOME=/home/crowbar/.crowbar \
    GIT_CONFIG_GLOBAL=/home/crowbar/.crowbar/gitconfig
WORKDIR /home/crowbar

# Holds the TLS identity and paired-client registry; losing it changes the
# fingerprint every paired client pinned. The image dirs are pre-chowned above
# so a fresh named volume inherits crowbar ownership.
VOLUME /home/crowbar/.crowbar
# Where repos created on the remote live, so they outlive the container.
VOLUME /home/crowbar/projects

EXPOSE 3737

ENTRYPOINT ["/sbin/tini", "--", "crowbar", "serve", "--host", "tcp://0.0.0.0:3737"]
