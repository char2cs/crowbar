# Side-by-side rig: the Tauri app (origin/develop) and the Rust/GPUI shell (this branch) on one
# seeded daemon, in an open, unauthenticated noVNC desktop. Build with build.sh, which stages
# stage/{crowbar,crowbar-api,crowbar-seed,crowbar-seed-chat,panes/,crowbar-desktop.deb} here.
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8 TZ=UTC
COPY stage/crowbar-desktop.deb /tmp/crowbar-desktop.deb
RUN apt-get update && apt-get install -y --no-install-recommends \
      xvfb openbox x11vnc novnc websockify tini xdotool x11-utils procps git ca-certificates \
      python3 python3-xlib python3-numpy python3-pil \
      mesa-vulkan-drivers libvulkan1 libxkbcommon-x11-0 libxcb-xkb1 libfontconfig1 libasound2t64 \
      fonts-dejavu-core fonts-noto-color-emoji \
      /tmp/crowbar-desktop.deb \
    && rm -rf /var/lib/apt/lists/* /tmp/crowbar-desktop.deb \
    && userdel -r ubuntu && useradd -m -u 1000 -s /bin/bash user
COPY stage/crowbar stage/crowbar-seed stage/crowbar-seed-chat /opt/crowbar/
COPY stage/crowbar-api /opt/crowbar/crowbar-api.real
COPY stage/panes /opt/crowbar/panes
COPY rig/rig rig/rigx.py rig/rig-proxy.py rig/crowbar-api rig/seed.sh /opt/rig/bin/
COPY rig/points.conf rig/rc.xml README.md /opt/rig/
COPY rig/gtk-settings.ini /home/user/.config/gtk-3.0/settings.ini
COPY index.html /usr/share/novnc/index.html
COPY entrypoint.sh /opt/rig/entrypoint.sh
# Tauri runs its sidecar from beside its binary: point it at the rig's per-app proxy wrapper.
RUN ln -sf /opt/rig/bin/crowbar-api /usr/bin/crowbar-api \
    && ln -s /opt/rig/bin/rig /usr/local/bin/rig \
    && install -o user -g user /dev/null /opt/rig/seeded-home.tar \
    && chown -R user:user /home/user/.config
ENV HOME=/home/user DISPLAY=:1 CROWBAR_HOME=/home/user/.crowbar-rig NO_AT_BRIDGE=1 \
    ZED_ALLOW_EMULATED_GPU=1 WEBKIT_DISABLE_DMABUF_RENDERER=1 WEBKIT_DISABLE_COMPOSITING_MODE=1 \
    CROWBAR_PANES_DIR=/opt/crowbar/panes CROWBAR_API_BIN=/opt/rig/bin/crowbar-api
USER user
WORKDIR /home/user
RUN /opt/rig/bin/seed.sh
EXPOSE 6080
ENTRYPOINT ["/usr/bin/tini", "--", "/opt/rig/entrypoint.sh"]
