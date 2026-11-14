#!/bin/sh
# Removes the login service installed by service-install.sh. Pairings, the TLS
# identity and all chats stay in ~/.crowbar, so a reinstall picks up where it left off.
set -eu

case $(uname -s) in
  Linux)
    systemctl --user disable --now crowbar.service 2>/dev/null || true
    rm -f "$HOME/.config/systemd/user/crowbar.service"
    systemctl --user daemon-reload 2>/dev/null || true
    ;;
  Darwin)
    label=com.crowbar.daemon
    launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
    rm -f "$HOME/Library/LaunchAgents/$label.plist"
    ;;
esac

rm -f "$HOME/.local/bin/crowbar"
echo "Crowbar daemon service removed."
