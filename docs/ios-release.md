# iOS release

`.github/workflows/ios-build.yml` is called by nightly, prerelease and stable-release.

- Apple secrets all present: signed `.ipa` (App Store Connect export), uploaded to TestFlight with `xcrun altool`.
- Any secret missing: unsigned simulator build; `Crowbar_<version>_ios-simulator.app.zip` is uploaded as the `ios-simulator` artifact and attached to the release. The job stays green.
- Marketing version: `desktop/src-tauri/Cargo.toml` version (`CFBundleShortVersionString`).
- Build number (`CFBundleVersion`): UTC `yymmddHHMM` of the build, passed as `bundle.iOS.bundleVersion`. One integer, unique and increasing across all three workflows.
- PR CI (`ios-checks` in `ci.yml`): `cargo check`/`clippy` for `aarch64-apple-ios-sim`, plugin Swift tests on an iPhone simulator, unsigned simulator build.

## One-time Apple setup

1. Apple Developer, Identifiers: register App ID `software.rabbyte.crowbar`.
2. App Store Connect, Apps: create the app record with that bundle ID (iOS). Add an internal TestFlight group.
3. Certificates: create an Apple Distribution certificate, export it with its private key as `.p12`.
4. Profiles: create an App Store distribution profile for the App ID and that certificate; download it.
5. App Store Connect, Users and Access, Integrations: create an API key with App Manager access; download `AuthKey_<KEYID>.p8` (once only); note Key ID and Issuer ID.

## Repository secrets

| Secret | Value |
| --- | --- |
| `APPLE_TEAM_ID` | 10-character team ID (shared with the macOS signing) |
| `IOS_CERTIFICATE` | `base64 -i dist.p12` |
| `IOS_CERTIFICATE_PASSWORD` | the `.p12` password |
| `IOS_MOBILE_PROVISION` | `base64 -i profile.mobileprovision` |
| `APP_STORE_CONNECT_API_KEY_ID` | API key ID |
| `APP_STORE_CONNECT_API_ISSUER_ID` | issuer ID |
| `APP_STORE_CONNECT_API_KEY` | `base64 -i AuthKey_<KEYID>.p8` |

The Tauri CLI reads `IOS_CERTIFICATE`, `IOS_CERTIFICATE_PASSWORD`, `IOS_MOBILE_PROVISION` and `APPLE_DEVELOPMENT_TEAM` (from `APPLE_TEAM_ID`) itself.

## gen/apple

Commit `gen/apple/{project.yml,Podfile,ExportOptions.plist,LaunchScreen.storyboard,Assets.xcassets,Sources,crowbar-desktop_iOS/{Info.plist,*.entitlements},crowbar-desktop.xcodeproj}` (minus `xcuserdata`). `assets/`, `Externals/`, `build/`, `Pods/`, `DerivedData/` are build output and ignored. `tauri ios init` overwrites `project.yml` and the launch assets; re-apply the hand edits after running it.

## Not verified locally

The signed path and TestFlight upload (no secrets), and the runner image's simulator list (the test step picks the first available iPhone).
