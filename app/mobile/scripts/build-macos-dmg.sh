#!/usr/bin/env bash
#
# Builds the universal macOS DMG for Crowbar.
#
# Signing + notarization are OPT-IN and driven entirely by environment
# variables. When the Apple signing secrets are present (i.e. the caller has
# configured the GitHub secrets), the produced bundle is signed with a
# Developer ID identity, notarized by Apple, and stapled — giving downloaders
# the benign "downloaded from the Internet" Gatekeeper prompt.
#
# When the secrets are absent (an unset GitHub secret expands to an empty
# string), this script runs the EXACT same `cargo tauri build` invocation that
# CI used before signing was wired up: an ad-hoc-signed (effectively unsigned)
# universal DMG. This is what makes the change a safe no-op until enrollment.
#
# Env vars consumed here directly:
#   APPLE_SIGNING_IDENTITY   e.g. "Developer ID Application: Name (TEAMID)".
#                            Its presence is the switch that turns signing on.
#
# Env vars consumed transparently by the Tauri bundler when signing is on:
#   APPLE_CERTIFICATE            base64-encoded .p12 (Tauri imports it into a
#                                temporary keychain automatically)
#   APPLE_CERTIFICATE_PASSWORD   password for the .p12
#   APPLE_ID / APPLE_PASSWORD / APPLE_TEAM_ID   notarization credentials
#                                (app-specific password). If unset, Tauri signs
#                                but skips notarization.
# When APPLE_SIGNING_IDENTITY is empty these are actively unset (see below) so
# an unconfigured CI run behaves exactly as it did before signing was wired up.
#
set -euo pipefail

# Resolve to desktop/src-tauri regardless of the caller's working directory.
cd "$(dirname "$0")/../src-tauri"

# --- Assemble the `bundle.macOS` config overlay -----------------------------
# Start empty; only add keys we actually need so the committed tauri.conf.json
# stays authoritative for everything else.
macos='{}'

# When the Liquid Glass Assets.car did not compile (older Xcode), drop the
# Resources/Assets.car file mapping so the bundler doesn't fail on a missing
# file. Mirrors the previous inline `--config '{"bundle":{"macOS":{"files":{}}}}'`.
if [ ! -f icons/Assets.car ]; then
  macos=$(jq -cn --argjson m "$macos" '$m + {files: {}}')
fi

# Turn on signing + hardened runtime ONLY when an identity is provided.
if [ -n "${APPLE_SIGNING_IDENTITY:-}" ]; then
  echo "==> Developer ID identity present: building SIGNED + notarized bundle."
  macos=$(jq -cn --argjson m "$macos" --arg id "$APPLE_SIGNING_IDENTITY" '
    $m + {
      signingIdentity: $id,
      hardenedRuntime: true,
      entitlements: "Entitlements.plist"
    }')
else
  echo "==> No APPLE_SIGNING_IDENTITY: building ad-hoc (unsigned) bundle."
  # CRITICAL: the Tauri bundler decides to sign based on the *presence* of the
  # APPLE_CERTIFICATE env var, NOT on our signingIdentity gate. GitHub Actions
  # exports an unset secret as a defined-but-empty string ("" != unset), which
  # is enough to make the bundler attempt `security import ""` and fail with
  # "SecKeychainItemImport: ... parameters ... not valid". Actively unset every
  # Apple var so the bundler sees the same clean environment it did before
  # signing was wired up. (No-op when the vars were never exported.)
  unset APPLE_CERTIFICATE APPLE_CERTIFICATE_PASSWORD APPLE_SIGNING_IDENTITY \
    APPLE_ID APPLE_PASSWORD APPLE_TEAM_ID
fi

# --- Build ------------------------------------------------------------------
args=(--target universal-apple-darwin --bundles dmg)
if [ "$macos" != '{}' ]; then
  args+=(--config "$(jq -cn --argjson m "$macos" '{bundle: {macOS: $m}}')")
fi

set -x
cargo tauri build "${args[@]}"
