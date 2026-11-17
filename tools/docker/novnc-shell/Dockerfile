# Open, unauthenticated noVNC desktop running the Rust/GPUI Crowbar shell and its daemon.
# Build context: this directory plus crowbar, crowbar-api and panes/ staged beside it (see build.sh).
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8
RUN apt-get update && apt-get install -y --no-install-recommends \
      xvfb openbox x11vnc novnc websockify tini xdotool scrot x11-utils git ca-certificates \
      libwebkit2gtk-4.1-0 libgtk-3-0t64 mesa-vulkan-drivers libvulkan1 libxkbcommon-x11-0 \
      libxcb-xkb1 libfontconfig1 libasound2t64 fonts-dejavu-core fonts-noto-color-emoji \
    && rm -rf /var/lib/apt/lists/* \
    && userdel -r ubuntu && useradd -m -u 1000 -s /bin/bash user
COPY crowbar crowbar-api /opt/crowbar/
COPY panes /opt/crowbar/panes
COPY entrypoint.sh /opt/crowbar/entrypoint.sh
COPY rc.xml /etc/xdg/openbox/rc.xml
COPY index.html /usr/share/novnc/index.html
ENV HOME=/home/user DISPLAY=:1 ZED_ALLOW_EMULATED_GPU=1 WEBKIT_DISABLE_DMABUF_RENDERER=1 \
    WEBKIT_DISABLE_COMPOSITING_MODE=1 CROWBAR_PANES_DIR=/opt/crowbar/panes \
    CROWBAR_API_BIN=/opt/crowbar/crowbar-api
USER user
WORKDIR /home/user
EXPOSE 6080
ENTRYPOINT ["/usr/bin/tini", "--", "/opt/crowbar/entrypoint.sh"]
