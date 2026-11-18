# Side-by-side rig: the Tauri app (origin/develop) and the Rust/GPUI shell (this branch) on one
# seeded daemon, in an open, unauthenticated noVNC desktop. Build with build.sh, which stages
# stage/{crowbar,crowbar-api,crowbar-seed,crowbar-seed-chat,panes/,crowbar-desktop.deb} here.
# The scripted provider (agent/), the forge CLI stand-ins (forge/), the seed profiles (seed/) and the
# rig driver (rig/) are added on top.
# The TypeScript language server the daemon runs for .ts files (it looks for it on PATH); only its
# node_modules are kept, on the image's own nodejs.
FROM node:22-bookworm-slim AS tsserver
RUN npm install -g --no-fund --no-audit typescript@5.6.3 typescript-language-server@4.3.3

FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8 TZ=UTC
COPY stage/crowbar-desktop.deb /tmp/crowbar-desktop.deb
RUN apt-get update && apt-get install -y --no-install-recommends \
      xvfb openbox x11vnc novnc websockify tini xdotool x11-utils procps git ca-certificates \
      python3 python3-xlib python3-numpy python3-pil \
      mesa-vulkan-drivers libvulkan1 libxkbcommon-x11-0 libxcb-xkb1 libfontconfig1 libasound2t64 \
      fonts-dejavu-core fonts-noto-color-emoji \
      xclip x11-xkb-utils dbus-daemon at-spi2-core python3-pyatspi ibus ibus-libpinyin nodejs \
      xdg-desktop-portal xdg-desktop-portal-gtk gnome-keyring \
      /tmp/crowbar-desktop.deb \
    && rm -rf /var/lib/apt/lists/* /tmp/crowbar-desktop.deb \
    && userdel -r ubuntu && useradd -m -u 1000 -s /bin/bash user \
    && mkdir -p /opt/rig/git-bin \
    && mv /usr/bin/git /usr/bin/git-receive-pack /usr/bin/git-shell /usr/bin/git-upload-archive \
          /usr/bin/git-upload-pack /usr/bin/scalar /opt/rig/git-bin/
COPY stage/crowbar stage/crowbar-seed stage/crowbar-seed-chat /opt/crowbar/
COPY stage/crowbar-api /opt/crowbar/crowbar-api.real
COPY stage/panes /opt/crowbar/panes
COPY rig/rig rig/rigx.py rig/rig-proxy.py rig/crowbar-api rig/rigapi.py rig/rigops.py rig/fixture-http.py \
     rig/a11y-dump.py rig/keyring-dump.py /opt/rig/bin/
COPY rig/points.conf rig/rc.xml README.md /opt/rig/
COPY agent/scripted-agent /usr/local/bin/scripted-agent
COPY forge/forge /opt/rig/bin/forge
COPY agent/scripted.yaml agent/scripted-lite.yaml /opt/rig/agent/
COPY agent/scenarios /opt/rig/agent/scenarios
COPY seed /opt/rig/seed
COPY --from=tsserver /usr/local/lib/node_modules/typescript /usr/local/lib/node_modules/typescript
COPY --from=tsserver /usr/local/lib/node_modules/typescript-language-server /usr/local/lib/node_modules/typescript-language-server
COPY rig/gtk-settings.ini /home/user/.config/gtk-3.0/settings.ini
COPY rig/portals.conf /home/user/.config/xdg-desktop-portal/portals.conf
COPY index.html /usr/share/novnc/index.html
COPY entrypoint.sh /opt/rig/entrypoint.sh
# Tauri runs its sidecar from beside its binary: point it at the rig's per-app proxy wrapper.
# gnome-keyring is not D-Bus activated: the Secret Service is up exactly while the rig runs it.
RUN ln -sf /opt/rig/bin/crowbar-api /usr/bin/crowbar-api \
    && rm /usr/share/dbus-1/services/org.freedesktop.secrets.service \
          /usr/share/dbus-1/services/org.gnome.keyring.service \
          /usr/share/dbus-1/services/org.freedesktop.impl.portal.Secret.service \
    && ln -s /opt/rig/bin/rig /usr/local/bin/rig \
    && ln -s /opt/rig/bin/forge /usr/local/bin/glab && ln -s /opt/rig/bin/forge /usr/local/bin/gh \
    && ln -s ../lib/node_modules/typescript-language-server/lib/cli.mjs /usr/local/bin/typescript-language-server \
    && ln -s ../lib/node_modules/typescript/bin/tsserver /usr/local/bin/tsserver \
    && install -d -o user -g user /opt/rig/profiles \
    && chown -R user:user /home/user/.config
# git lives only in /opt/rig/git-bin, so `rig daemon nogit` can take it off the daemon's PATH (the
# daemon also looks in /usr/bin and /usr/local/bin when PATH has no git).
ENV PATH=/opt/rig/git-bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    HOME=/home/user DISPLAY=:1 CROWBAR_HOME=/home/user/.crowbar-rig NO_AT_BRIDGE=1 \
    ZED_ALLOW_EMULATED_GPU=1 WEBKIT_DISABLE_DMABUF_RENDERER=1 WEBKIT_DISABLE_COMPOSITING_MODE=1 \
    CROWBAR_PANES_DIR=/opt/crowbar/panes CROWBAR_API_BIN=/opt/rig/bin/crowbar-api
USER user
WORKDIR /home/user
RUN /opt/rig/seed/build.sh
EXPOSE 6080
ENTRYPOINT ["/usr/bin/tini", "--", "/opt/rig/entrypoint.sh"]
