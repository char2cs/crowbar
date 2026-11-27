FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends build-essential cmake clang pkg-config curl ca-certificates git libfontconfig-dev libfreetype-dev libxcb1-dev libxkbcommon-dev libxkbcommon-x11-dev libwayland-dev libssl-dev libvulkan-dev libx11-dev libx11-xcb-dev libxcb-xkb-dev libxkbfile-dev libdbus-1-dev libzstd-dev libasound2-dev libxcb-render0-dev libxcb-shape0-dev libxcb-xfixes0-dev xvfb x11-utils xdotool imagemagick mesa-vulkan-drivers vulkan-tools libgl1-mesa-dri libegl1 openbox fonts-dejavu-core libwebkit2gtk-4.1-dev libgtk-3-dev libxdo-dev librsvg2-dev patchelf file && rm -rf /var/lib/apt/lists/*
ENV RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo PATH=/usr/local/cargo/bin:$PATH
RUN curl -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.100.0 --profile minimal --component clippy,rustfmt
WORKDIR /w
