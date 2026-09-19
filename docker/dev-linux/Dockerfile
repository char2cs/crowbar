# Linux desktop dev/verification box for Crowbar's Tauri app.
#
# This is NOT the production Linux distribution artifact — it's a disposable
# sandbox with a real X11 desktop, exposed over noVNC, so the app can be built
# and visually driven on Linux without a physical Linux+GUI machine. The
# webview-backend strategy for shipping Crowbar on Linux (system webkit2gtk
# vs. a bundled engine) is decided separately; this box just needs to run
# whatever gets built.
#
# Base: ubuntu:24.04 — matches the generation GitHub Actions' `ubuntu-latest`
# runners use for the existing tauri-checks-linux CI job, so `apt` package
# names/versions (webkit2gtk-4.1, etc.) line up with what CI already proves
# builds.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    DISPLAY=:1 \
    RESOLUTION=1280x800x24 \
    VNC_PORT=5900 \
    NOVNC_PORT=6080

# --- Desktop environment + remote access ---------------------------------
# xfce4 (+ xfce4-terminal): the standard lightweight DE for VNC boxes — a
# window manager and panel, not the full xfce4-goodies bundle.
# Xvfb: virtual framebuffer X server (no physical display in a container).
# x11vnc: VNC server that shares an existing X display (Xvfb's).
# python3-websockify: bridges noVNC's browser websocket to the VNC TCP port.
RUN apt-get update && apt-get install -y --no-install-recommends \
    xfce4 \
    xfce4-terminal \
    xvfb \
    x11vnc \
    x11-utils \
    dbus-x11 \
    python3-websockify \
    git \
    curl \
    wget \
    unzip \
    ca-certificates \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# noVNC itself (the browser client + `novnc_proxy` launcher) isn't packaged
# in Ubuntu's repos in a layout `python3-websockify` expects, so it's pulled
# straight from upstream at a pinned release tag.
ARG NOVNC_VERSION=1.5.0
RUN git clone --branch v${NOVNC_VERSION} --depth 1 \
    https://github.com/novnc/noVNC.git /opt/novnc \
    && ln -s /opt/novnc/vnc.html /opt/novnc/index.html

# --- Tauri v2 Linux build prerequisites -----------------------------------
# Matches the official Tauri v2 Linux prereqs list; CI's tauri-checks-linux
# job only installs the first three because GitHub's ubuntu-latest runner
# image already ships build-essential/curl/wget/file/patchelf/pkg-config —
# this is a bare Ubuntu base, so all of it has to be explicit here.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libwebkit2gtk-4.1-dev \
    libayatana-appindicator3-dev \
    librsvg2-dev \
    libgtk-3-dev \
    libssl-dev \
    libxdo-dev \
    build-essential \
    file \
    patchelf \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# --- Toolchains, pinned to match this repo -------------------------------
# Go: api/go.mod / CI's go-version. Architecture-detected so this also
# builds natively on arm64 hosts (e.g. Apple Silicon) instead of only amd64.
ARG GO_VERSION=1.26.2
RUN GOARCH_DEB="$(dpkg --print-architecture)" \
    && curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH_DEB}.tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"

# Rust: CI's dtolnay/rust-toolchain pin in .github/workflows/ci.yml.
ARG RUST_VERSION=1.88
ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH="/usr/local/cargo/bin:${PATH}"
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | \
    sh -s -- -y --default-toolchain ${RUST_VERSION} --profile default \
    && rustup component add rustfmt clippy

# Bun: CI installs via oven-sh/setup-bun with no version pin, so this
# matches that (latest) rather than inventing a pin CI doesn't have.
RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:${PATH}"

WORKDIR /workspace

COPY docker/dev-linux/start.sh /usr/local/bin/start.sh
RUN chmod +x /usr/local/bin/start.sh

EXPOSE 6080

ENTRYPOINT ["/usr/local/bin/start.sh"]
