#!/usr/bin/env bash
# Install the release iOS app on a device (free Apple ID) or a simulator.
# Usage: ios-install.sh device|sim|doctor
# Env: APPLE_TEAM_ID, IOS_BUNDLE_ID (or BUNDLE_ID_SUFFIX), DEVICE_ID, SIM_ID, YES=1
set -euo pipefail

MODE="${1:-doctor}"
DRY_RUN="${DRY_RUN:-}"
[ "${2:-}" = --dry-run ] && DRY_RUN=1
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TAURI_DIR="$ROOT/desktop/src-tauri"
BASE_ID="software.rabbyte.crowbar"
BUNDLE_ID="${IOS_BUNDLE_ID:-${BUNDLE_ID_SUFFIX:+$BASE_ID.$BUNDLE_ID_SUFFIX}}"
BUNDLE_ID="${BUNDLE_ID:-$BASE_ID}"
MIN_IOS=26
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
FAILS=0

ok() { printf '  [ ok ] %s\n' "$*"; }
bad() { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS + 1)); }
die() { printf '\nerror: %s\n' "$*" >&2; exit 1; }
steps() { # steps "title" "1. ..." ...
  printf '\n  To fix: %s\n' "$1"; shift
  printf '    %s\n' "$@"
}

# Run a state-changing command, or only print it under --dry-run.
run() {
  if [ -n "$DRY_RUN" ]; then printf '  [dry-run] %s\n' "$*"; return 0; fi
  "$@"
}

confirm() {
  [ "${YES:-}" = 1 ] && return 0
  [ -t 0 ] || return 1
  read -r -p "  $1 [y/N] " a
  [ "$a" = y ] || [ "$a" = Y ]
}

# devicectl JSON helpers (python3 ships with the Xcode CLT)
json() { python3 -I -c "$1" "$2"; }

check_toolchain() {
  echo "Toolchain"
  if xcode-select -p >/dev/null 2>&1 && xcodebuild -version >/dev/null 2>&1; then
    ok "Xcode $(xcodebuild -version | head -1 | cut -d' ' -f2) ($(xcode-select -p))"
  else
    bad "Xcode / command line tools not usable"
    steps "install Xcode" "1. Install Xcode from the App Store, open it once, accept the license" \
      "2. sudo xcode-select -s /Applications/Xcode.app/Contents/Developer"
  fi
  local want="aarch64-apple-ios"
  [ "$MODE" = sim ] && want="aarch64-apple-ios-sim"
  [ "$MODE" = doctor ] && want="aarch64-apple-ios aarch64-apple-ios-sim"
  local t installed
  installed="$(rustup target list --installed 2>/dev/null || true)"
  for t in $want; do
    if grep -qx "$t" <<<"$installed"; then
      ok "rust target $t"
    elif confirm "rust target $t missing; run 'rustup target add $t'?" && run rustup target add "$t" >/dev/null; then
      ok "rust target $t (installed now)"
    else
      bad "rust target $t missing (rustup target add $t)"
    fi
  done
  if cargo tauri --version >/dev/null 2>&1; then ok "cargo tauri CLI"
  else bad "cargo tauri CLI missing (cargo install tauri-cli --version '^2')"; fi
  if command -v bun >/dev/null 2>&1; then ok "bun $(bun --version)"
  else bad "bun missing (https://bun.sh)"; fi
}

# Echo "<udid>|<name>|<os>" of the first connected, paired iPhone.
find_device() {
  local out="$WORK/devices.json"
  xcrun devicectl list devices --json-output "$out" >/dev/null 2>&1 || return 1
  json '
import json, sys
for d in json.load(open(sys.argv[1]))["result"]["devices"]:
    hw, cp, dp = d.get("hardwareProperties", {}), d.get("connectionProperties", {}), d.get("deviceProperties", {})
    if hw.get("platform") != "iOS" or hw.get("deviceType", "iPhone") != "iPhone":
        continue
    if cp.get("pairingState") != "paired":
        continue
    print("|".join([hw.get("udid", d["identifier"]), dp.get("name", "iPhone"), dp.get("osVersionNumber", "0")]))
    break
' "$out"
}

detect_team() {
  [ -n "${APPLE_TEAM_ID:-}" ] && { echo "$APPLE_TEAM_ID"; return; }
  local subj
  subj="$(security find-certificate -c "Apple Development" -p 2>/dev/null \
    | openssl x509 -noout -subject -nameopt multiline 2>/dev/null || true)"
  awk -F'= *' '/organizationalUnitName/ {print $2; exit}' <<<"$subj" | tr -d ' '
}

check_device() {
  echo "Device"
  DEV=""
  DEV="$(find_device || true)"
  if [ -z "$DEV" ]; then
    bad "no paired iPhone found"
    steps "connect the phone" "1. Plug the iPhone into this Mac with a cable and unlock it" \
      "2. Tap 'Trust This Computer' on the phone and enter the passcode" \
      "3. Phone: Settings > Privacy & Security > Developer Mode > On (the phone restarts; confirm 'Turn On' after reboot)" \
      "4. Re-run: make ios-doctor"
    return
  fi
  local udid name os
  IFS='|' read -r udid name os <<<"$DEV"
  DEVICE_UDID="${DEVICE_ID:-$udid}"
  ok "iPhone '$name' ($DEVICE_UDID), iOS $os"
  if [ "${os%%.*}" -lt "$MIN_IOS" ]; then
    bad "iOS $os is older than required $MIN_IOS (update the phone: Settings > General > Software Update)"
  fi
  local mode
  mode="$(xcrun devicectl device info details --device "$DEVICE_UDID" --json-output "$WORK/d.json" >/dev/null 2>&1 \
    && json 'import json,sys;print(json.load(open(sys.argv[1]))["result"]["deviceProperties"].get("developerModeStatus",""))' "$WORK/d.json" || true)"
  if [ "$mode" = enabled ]; then ok "Developer Mode enabled"
  else bad "Developer Mode is '${mode:-unknown}': Settings > Privacy & Security > Developer Mode > On, then restart"; fi
}

check_signing() {
  echo "Signing"
  TEAM="$(detect_team)"
  if [ -n "$TEAM" ]; then
    ok "team $TEAM (Apple Development certificate; override with APPLE_TEAM_ID)"
  else
    bad "no 'Apple Development' signing identity"
    steps "sign in to Xcode with your Apple ID" "1. Xcode > Settings > Accounts > + > Apple ID, sign in" \
      "2. Select the account > Manage Certificates > + > Apple Development" \
      "3. Re-run: make ios-doctor (or set APPLE_TEAM_ID=<10-char Team ID>)"
  fi
  [ "$BUNDLE_ID" = "$BASE_ID" ] || ok "bundle id override: $BUNDLE_ID"
}

check_web() {
  [ "$MODE" = doctor ] && return
  echo "Web bundle"
  if (cd "$ROOT/web" && run bun install --frozen-lockfile >/dev/null && run bun run build >/dev/null); then
    ok "web/dist built"
  else die "web build failed; run 'cd web && bun run build' to see why"; fi
}

tauri_build() { # tauri_build <target> [extra args]
  local target="$1"; shift
  local cfg='{}'
  [ "$BUNDLE_ID" = "$BASE_ID" ] || cfg="{\"identifier\":\"$BUNDLE_ID\"}"
  echo "Building (release, target $target)"
  (cd "$TAURI_DIR" && run cargo tauri ios build --ci --target "$target" --config "$cfg" "$@") \
    || die "cargo tauri ios build failed (see output above)$(
      [ -n "${TEAM:-}" ] && printf '\n  If it says the bundle id is unavailable, retry with BUNDLE_ID_SUFFIX=<yourname>')"
}

find_app() { # newest Crowbar .app under the Tauri iOS build outputs
  find "$TAURI_DIR/gen/apple/build" -name '*.app' -path "$1" -type d -print0 2>/dev/null \
    | xargs -0 ls -dt 2>/dev/null | head -1
}

do_sim() {
  local sim="${SIM_ID:-booted}"
  xcrun simctl list devices | grep -q '(Booted)' || [ "$sim" != booted ] \
    || die "no booted simulator; boot one (xcrun simctl boot <udid>) or set SIM_ID=<udid>"
  tauri_build aarch64-sim
  local app
  app="$(find_app '*xcarchive*')"
  [ -n "$app" ] || app="$(find_app '*')"
  [ -n "$DRY_RUN" ] && app="${app:-<built Crowbar.app>}"
  [ -n "$app" ] || die "build produced no .app under desktop/src-tauri/gen/apple/build"
  local id
  id="$BUNDLE_ID"
  [ -n "$DRY_RUN" ] || id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$app/Info.plist")"
  run xcrun simctl install "$sim" "$app" || die "simctl install failed"
  run xcrun simctl launch "$sim" "$id" || die "simctl launch failed"
  if [ -n "$DRY_RUN" ]; then echo "  [dry-run] would install/launch $id on simulator $sim"
  else ok "launched $id on simulator $sim"; fi
}

do_device() {
  [ -n "$DRY_RUN" ] || [ -n "${DEV:-}" ] || die "preflight failed; fix the items above (make ios-doctor)"
  export APPLE_DEVELOPMENT_TEAM="${TEAM:-<team>}"
  DEVICE_UDID="${DEVICE_UDID:-<device>}"
  tauri_build aarch64 --export-method debugging
  local app
  app="$(find_app '*xcarchive*')"
  [ -n "$DRY_RUN" ] && app="${app:-<signed Crowbar.app>}"
  [ -n "$app" ] || die "no signed .app in the archive; check signing errors above"
  run xcrun devicectl device install app --device "$DEVICE_UDID" "$app" \
    || die "install failed; is the phone unlocked and still connected?"
  local deadline=$((SECONDS + 30))
  until run xcrun devicectl device process launch --device "$DEVICE_UDID" "$BUNDLE_ID" >/dev/null 2>"$WORK/launch.err"; do
    if grep -qi 'profile.*not.*trusted\|invalid code signature\|not been explicitly trusted' "$WORK/launch.err" \
      || [ "$SECONDS" -ge "$deadline" ]; then
      steps "trust the developer profile (first install only)" \
        "1. Phone: Settings > General > VPN & Device Management" \
        "2. Tap your Apple ID under 'Developer App' > Trust" \
        "3. Re-run: make ios-device"
      die "could not launch $BUNDLE_ID"
    fi
  done
  if [ -n "$DRY_RUN" ]; then echo "  [dry-run] would install/launch $BUNDLE_ID"
  else ok "installed and launched $BUNDLE_ID"; fi
  cat <<EOF

Next steps
  1. Install Tailscale on the phone and sign in to the same tailnet as this Mac.
  2. A free Apple ID profile expires after 7 days; re-run 'make ios-device' to reinstall.
EOF
}

case "$MODE" in
  doctor | device | sim) ;;
  *) die "usage: ios-install.sh device|sim|doctor" ;;
esac

echo "Crowbar iOS ($MODE)"
check_toolchain
if [ "$MODE" != sim ]; then check_device; check_signing; fi
if [ "$MODE" = doctor ]; then
  echo
  [ "$FAILS" -eq 0 ] || { echo "$FAILS check(s) failed."; exit 1; }
  echo "All checks passed."
  exit 0
fi
[ "$FAILS" -eq 0 ] || [ -n "$DRY_RUN" ] || die "preflight failed; fix the items above"
check_web
if [ "$MODE" = sim ]; then do_sim; else do_device; fi
