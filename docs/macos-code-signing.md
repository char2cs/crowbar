# macOS Code Signing & Notarization

The three release workflows (`nightly.yml`, `prerelease.yml`, `stable-release.yml`) and the
`package` job of `rust.yml` build the macOS DMG through `.github/workflows/desktop-build.yml`,
which runs `app/desktop/packaging/package.sh`: it lipos the arm64 and x86_64 builds of the
`crowbar` shell and the `crowbar-api` daemon into one universal bundle and packages it with
cargo-packager.

Signing + notarization is **opt-in via GitHub secrets**. Until the six secrets
below exist, every build produces an ad-hoc-signed (effectively unsigned) DMG. The
moment the secrets are present, builds are signed with a Developer ID identity, notarized by Apple, and stapled — giving downloaders the
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

## First signed run: validate the daemon

The bundle carries a second executable, the Go daemon, at `Contents/MacOS/crowbar-api`.
cargo-packager signs every Mach-O file in the bundle inside out with the Hardened Runtime and
a secure timestamp before it signs the bundle, so notarization *should* accept it, but **the
first signed build is the moment to confirm it**. If notarization rejects the bundle:

1. Read the notarization log: the CI output names the rejected path.
2. Check the daemon's signature:
   `codesign -dvvv --entitlements - <path-to-Crowbar.app>/Contents/MacOS/crowbar-api`.

Recommended: run `app/desktop/packaging/package.sh` once on a Mac with the env vars set
before relying on CI; signing errors are far easier to read in a local terminal.

## Files involved

- `app/desktop/packaging/package.sh` builds and packages; it turns signing on only when
  `APPLE_SIGNING_IDENTITY` is set, and otherwise ad-hoc signs the bundle with `adhoc-sign.sh`.
- `app/desktop/packaging/packager.json`: the cargo-packager configuration.
- `app/desktop/packaging/Entitlements.plist`: Hardened Runtime entitlements (WebView JIT).
- `.github/workflows/desktop-build.yml`: passes the six secrets into the packaging step.
