#!/bin/sh
# Installs the Crowbar daemon as a per-user login service that serves paired
# remote clients over TLS. Usage: service-install.sh <release-tag>
set -eu

ref=${1:?usage: service-install.sh <release-tag>}
port=${CROWBAR_PORT:-3737}
bin_dir=$HOME/.local/bin
bin=$bin_dir/crowbar

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

mkdir -p "$bin_dir"
curl -fsSL -o "$bin.new" "https://github.com/char2cs/crowbar/releases/download/$ref/crowbar-api-$os-$arch"
chmod +x "$bin.new"
mv -f "$bin.new" "$bin"

case $os in
  linux)
    unit_dir=$HOME/.config/systemd/user
    mkdir -p "$unit_dir"
    cat > "$unit_dir/crowbar.service" <<EOF
[Unit]
Description=Crowbar daemon

[Service]
ExecStart=$bin serve --host tcp://0.0.0.0:$port
Restart=on-failure

[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload
    systemctl --user enable crowbar.service
    systemctl --user restart crowbar.service
    # Without linger a user service only runs while that user is logged in.
    loginctl enable-linger "$(id -un)" 2>/dev/null || true
    ;;
  darwin)
    label=com.crowbar.daemon
    plist=$HOME/Library/LaunchAgents/$label.plist
    mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
    cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key>
  <array>
    <string>$bin</string>
    <string>serve</string>
    <string>--host</string>
    <string>tcp://0.0.0.0:$port</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardErrorPath</key><string>$HOME/Library/Logs/crowbar.log</string>
</dict>
</plist>
EOF
    domain=gui/$(id -u)
    launchctl bootout "$domain/$label" 2>/dev/null || true
    launchctl bootstrap "$domain" "$plist"
    ;;
  *)
    echo "unsupported OS: $os" >&2
    exit 1
    ;;
esac

echo "Crowbar daemon running on port $port. Run 'crowbar pair' on this machine to get a pairing code."
