#!/usr/bin/env bash
# Builds the desktop packages into app/target/package/dist with the release asset names:
# Crowbar_<version>_universal.dmg on macOS, Crowbar_<version>_<arch>.AppImage and .deb on Linux.
# The version is the Rust workspace's. APPLE_SIGNING_IDENTITY (with APPLE_CERTIFICATE,
# APPLE_CERTIFICATE_PASSWORD, APPLE_ID, APPLE_PASSWORD, APPLE_TEAM_ID) signs and notarizes the DMG.
# VITE_BUILD_* reach the pane pages. Needs cargo-packager, go, bun and jq.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
app=$(cd "$here/../.." && pwd)
repo=$(dirname "$app")
icons=$app/mobile/src-tauri/icons
work=$app/target/package
stage=$work/stage
out=$work/out
dist=$work/dist
mac_targets=(aarch64-apple-darwin x86_64-apple-darwin)

workspace_version() {
  sed -n '/^\[workspace\.package\]/,/^\[/s/^version = "\(.*\)"/\1/p' "$app/Cargo.toml"
}

fresh_stage() {
  rm -rf "$stage" "$out" "$dist"
  mkdir -p "$stage/bin" "$stage/icons" "$out" "$dist"
  cp "$here"/{crowbar.desktop,Info.plist,Entitlements.plist,adhoc-sign.sh} "$stage/"
  cp "$icons"/{32x32.png,128x128.png,128x128@2x.png,icon.icns} "$stage/icons/"
}

build_panes() {
  (cd "$app/panes" && bun install --frozen-lockfile && bun run build:panes)
  cp -R "$app/panes/dist-panes" "$stage/panes"
}

build_daemon() {
  local goos=$1 goarch=$2 cgo=$3 output=$4
  (cd "$repo/api" && GOOS=$goos GOARCH=$goarch CGO_ENABLED=$cgo go build -tags noEmbed -o "$output" ./cmd/crowbar)
}

build_shell() {
  cargo build --locked --release -p crowbar --manifest-path "$app/Cargo.toml" "$@"
}

write_config() {
  local version=$1 filter=$2
  jq --arg version "$version" ".version = \$version | $filter" "$here/packager.json" >"$stage/packager.json"
}

run_packager() {
  cargo packager --verbose --config "$stage/packager.json" --out-dir "$out"
}

deb_depends() {
  local scratch
  scratch=$(mktemp -d)
  mkdir "$scratch/debian"
  printf 'Source: crowbar\n\nPackage: crowbar\nArchitecture: any\n' >"$scratch/debian/control"
  (cd "$scratch" && dpkg-shlibdeps -O "$stage/bin/crowbar") | sed -n 's/^shlibs:Depends=//p' | tr ',' '\n' | sed 's/^ *//'
  echo libvulkan1
  echo xdg-utils
  rm -rf "$scratch"
}

package_linux() {
  local version=$1 machine goarch deb_arch label
  machine=$(uname -m)
  case $machine in
    x86_64) goarch=amd64 deb_arch=amd64 label=amd64 ;;
    aarch64) goarch=arm64 deb_arch=arm64 label=aarch64 ;;
    *) echo "unsupported architecture: $machine" >&2 && return 1 ;;
  esac
  build_shell
  cp "$app/target/release/crowbar" "$stage/bin/crowbar"
  build_daemon linux "$goarch" 0 "$stage/bin/crowbar-api"
  deb_depends >"$stage/depends.txt"
  write_config "$version" '.formats = ["deb", "appimage"]'
  DEPLOY_GTK_VERSION=3 run_packager
  cp "$out/crowbar_${version}_${deb_arch}.deb" "$dist/Crowbar_${version}_${deb_arch}.deb"
  cp "$out/crowbar_${version}_${machine}.AppImage" "$dist/Crowbar_${version}_${label}.AppImage"
}

lipo_shell() {
  local target slices=()
  rustup target add "${mac_targets[@]}"
  for target in "${mac_targets[@]}"; do
    build_shell --target "$target"
    slices+=("$app/target/$target/release/crowbar")
  done
  lipo -create "${slices[@]}" -output "$stage/bin/crowbar"
}

lipo_daemon() {
  build_daemon darwin arm64 1 "$work/crowbar-api-arm64"
  build_daemon darwin amd64 1 "$work/crowbar-api-amd64"
  lipo -create "$work/crowbar-api-arm64" "$work/crowbar-api-amd64" -output "$stage/bin/crowbar-api"
}

liquid_glass_icon() {
  bash "$app/mobile/icons-compiler/compile-icon.sh" >&2
  [[ -f $icons/Assets.car ]] || return 0
  cp "$icons/Assets.car" "$stage/icons/Assets.car"
  echo '| .resources += [{"src": "icons/Assets.car", "target": "Assets.car"}]'
}

signing() {
  if [[ -n ${APPLE_SIGNING_IDENTITY:-} ]]; then
    echo '| .macos.signingIdentity = env.APPLE_SIGNING_IDENTITY | .macos.entitlements = "Entitlements.plist"'
    return
  fi
  echo '| .beforeEachPackageCommand = "bash adhoc-sign.sh"'
}

package_macos() {
  local version=$1 filter
  lipo_shell
  lipo_daemon
  filter=".formats = [\"app\", \"dmg\"] | .targetTriple = \"universal-apple-darwin\" $(liquid_glass_icon) $(signing)"
  write_config "$version" "$filter"
  APP_BUNDLE="$out/Crowbar.app" run_packager
  cp "$out/Crowbar_${version}_universal.dmg" "$dist/"
}

main() {
  local version
  version=$(workspace_version)
  fresh_stage
  build_panes
  case $(uname -s) in
    Linux) package_linux "$version" ;;
    Darwin) package_macos "$version" ;;
    *) echo "unsupported system: $(uname -s)" >&2 && return 1 ;;
  esac
  ls -l "$dist"
}

main "$@"
