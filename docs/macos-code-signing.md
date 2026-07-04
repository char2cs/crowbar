# macOS Code Signing & Notarization

Crowbar's macOS DMGs are built by three workflows — `nightly.yml`, `prerelease.yml`,
`stable-release.yml` — all of which call `desktop/scripts/build-macos-dmg.sh`.

Signing + notarization is **opt-in via GitHub secrets**. Until the six secrets
below exist, every build produces the same ad-hoc-signed (effectively unsigned)
DMG it always has. The moment the secrets are present, builds are signed with a
Developer ID identity, notarized by Apple, and stapled — giving downloaders the
benign _"Crowbar is an app downloaded from the Internet. Are you sure you want to
open it?"_ Gatekeeper prompt instead of the "unidentified developer" block.

## Why this requires the paid Apple Developer Program

There is **no free path** to the friendly prompt. The Gatekeeper prompt that just
needs a click is produced only by a **notarized** app, and notarization requires a
**Developer ID Application** certificate, which is only issued to **paid** Apple
Developer Program members ($99/yr). A free Apple ID can sign apps for local testing
but is refused by Apple's notary service ("Team is not enrolled in the Apple
Developer Program"). On macOS Sequoia (15+), an un-notarized download can no longer
be opened via right-click → Open; the user must dig through System Settings →
Privacy & Security. Notarization is what avoids that.

## One-time setup (after enrolling in the Apple Developer Program)

### 1. Create a Developer ID Application certificate

- Xcode → Settings → Accounts → your team → **Manage Certificates** → `+` →
  **Developer ID Application**. (Or create it at
  <https://developer.apple.com/account/resources/certificates>.)

### 2. Export it as a `.p12`

- Keychain Access → **My Certificates** → right-click the
  _"Developer ID Application: … (TEAMID)"_ entry (expand it so the private key is
  included) → **Export** → `.p12`, set a password.

### 3. Base64-encode the `.p12`

```sh
base64 -i certificate.p12 | pbcopy   # now on your clipboard
```

### 4. Create an app-specific password for notarization

- <https://account.apple.com> → Sign-In and Security → **App-Specific Passwords**
  → generate one (label it e.g. "crowbar-notary").

### 5. Find your identity string and Team ID

```sh
security find-identity -v -p codesigning   # copy the "Developer ID Application: …" line
```

The Team ID is the 10-character code in parentheses (also shown at
<https://developer.apple.com/account> → Membership).

## GitHub secrets to add

Repo → Settings → Secrets and variables → Actions → **New repository secret**:

| Secret | Value |
| --- | --- |
| `APPLE_CERTIFICATE` | base64 of the `.p12` (step 3) |
| `APPLE_CERTIFICATE_PASSWORD` | the password you set when exporting the `.p12` |
| `APPLE_SIGNING_IDENTITY` | e.g. `Developer ID Application: Your Name (TEAMID)` — **the switch that turns signing on** |
| `APPLE_ID` | your Apple ID email |
| `APPLE_PASSWORD` | the app-specific password (step 4) — **not** your Apple ID password |
| `APPLE_TEAM_ID` | your 10-character Team ID |

That's it — the next build on any of the three workflows signs + notarizes
automatically. No code changes needed.

## First signed run: validate the sidecar

Crowbar bundles a Go sidecar (`crowbar-api`) as an `externalBin`. Bundling a
sidecar is the one part of Tauri macOS signing with a history of sharp edges
(tauri-apps/tauri#11992 — "the signature of the binary is invalid" during
notarization). Tauri v2's bundler signs nested binaries as part of bundle signing,
so this *should* work out of the box, but **the first signed build is the moment to
confirm it**. If notarization rejects the bundle:

1. Read the notarization log — the CI output prints a URL / `xcrun notarytool log`
   command with the exact rejected path.
2. Confirm the sidecar got the Hardened Runtime + a secure timestamp:
   `codesign -dvvv --entitlements - <path-to-Crowbar.app>/Contents/Resources/... `
   and check the sidecar under `Contents/MacOS` / `Contents/Resources`.
3. If the sidecar is the culprit, the usual fix is to pre-sign it before bundling
   (`codesign --force --options runtime --timestamp -s "$APPLE_SIGNING_IDENTITY"
   binaries/crowbar-api-universal-apple-darwin`) in the "Build Go sidecar" step, or
   bump the pinned Tauri version.

Recommended: do one **manual** local `cargo tauri build` with the env vars set
before relying on CI — signing errors are far easier to read in a local terminal.

## Files involved

- `desktop/scripts/build-macos-dmg.sh` — assembles the signing `--config` overlay
  and runs the build; no-op signing when `APPLE_SIGNING_IDENTITY` is empty.
- `desktop/src-tauri/Entitlements.plist` — Hardened Runtime entitlements (WebView
  JIT). Required for a notarized Tauri app to launch without crashing.
- `.github/workflows/{nightly,prerelease,stable-release}.yml` — pass the six
  secrets into the build step's environment.
