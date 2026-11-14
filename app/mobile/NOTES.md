# iOS shell: what is verified and what is not

Mobile stays Tauri + Swift. Layout: `ios/` (Swift package), `ios-plugin/` (Rust Tauri plugin that bridges it), `src-tauri/` (app crate, its own Cargo workspace and lockfile), `icons-compiler/`, `icons-src/`, `scripts/`.

## Paths that changed

- `ios-plugin/build.rs` points the plugin at `../ios`. The tauri-plugin build script copies `tauri-api` to `<ios path parent>/.tauri`, so it now lands in `app/mobile/.tauri` (git-ignored); `ios/Package.swift` still reaches it as `../.tauri/tauri-api`.
- `src-tauri/Cargo.toml` depends on `../ios-plugin`; `.github/workflows/ci.yml` runs `xcodebuild test` in `app/mobile/ios`.
- `ios-plugin/scripts/gen-icons.mjs` writes `ios/Sources/CrowbarIcon.swift` (it was `CrowbarIconData.swift`; the generated file now holds only the `CrowbarIcon` enum, `CrowbarIconArt` and the rendering extension are hand-written files).

## Not verified here (no Mac, no Xcode)

Everything that imports UIKit, SwiftUI, WebKit or Tauri was edited mechanically and reviewed, not compiled. The macOS CI job (`ios-checks`) and a device run must cover:

- Compilation of the whole package after the one-type-per-file split (private types became internal; `fileprivate func step` became internal).
- Nested row views that replaced helper `func`s in `ModelPickerSheet`, `HostListSection`, `PlusMenuSheet`, `AttachSheet`, `ConnectFields`, `SettingsSheet` and `AppearanceCards` (the split-card diagonal is now a `Path` in a `GeometryReader` mask instead of a `Shape`). Check on device: list rows still get their insets and swipe actions, the Appearance split card, the Settings push rows and destinations.
- `AppScheme` is no longer a singleton: `ChromeOverlays` owns one instance and passes it to `SecondaryPage.make(scheme:)`.
- `NavigationMachine.endDrag(velocity:ending:)` and `SheetPresenter.update(_:dismissal:over:)` replaced Bool parameters; `ChromeIntent.start` and friends are `static let`.
- New XCTests that touch UIKit (`KeyboardFitTests`, `NavigationControllerTests`, `SheetPresenterTests`, `StripCatcherTests`, `ModelPickerModelTests`) were written from the API, never run. `KeyboardFitTests` assumes `UIView.animate(withDuration: 0)` applies its changes synchronously and that `convert(_:from: nil)` is the identity for a view outside a window.
- `swiftlint --strict` (config in `ios/.swiftlint.yml`, including the analyzer rules) was not run: the binary could not be fetched here. Expected remaining findings: `no_magic_numbers` in views, `function_body_length` in a few view bodies.
- `swift-format lint --strict` was run (Swift 6.0.3 toolchain in Docker) and is clean for every file except the generated `Sources/CrowbarIcon.swift` (very long string literals; `LineLength`).
- `node ios-plugin/scripts/gen-icons.mjs --check` needs `app/panes/node_modules`; the generator output was edited by hand to match the committed file.

## Verified here

- `tools/scripts/check-swift-rules.sh --self-test` passes. On `ios/` the structure rules are clean except the `test-missing` findings listed below.
- The Foundation-only part of the package (the data types, `NavigationMachine`, `SheetPlan`, `EffortDial`) and its tests compile and pass on Linux with Swift 6.0.3 in Docker (35 tests; `CSSArgs`, `ConnectDrafts` and `HostDot` need SwiftUI or Tauri and were not run).
- The Rust plugin's library target passed `cargo clippy -- -D warnings` with the `app/` lint table on Linux (host target, `c-rust` image). After that run the test fixtures were changed (`FrontendAsset`, because `tauri::Asset` is `#[non_exhaustive]` and cannot be built in tests) and `cargo fmt --check`, the plugin's unit tests and the clippy run on test targets could not be repeated: the Docker daemon stopped starting containers. Run `cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test` in `app/mobile/ios-plugin` first. The `target_os = "ios"` branch (`register_swift_plugin`, `ios_plugin_binding!`) is compiled only by the macOS CI job.

## Findings the checkers still report, on purpose

- `test-missing` for files whose only functions are UIKit, SwiftUI, WebKit or Tauri glue with nothing to assert without a device: `AppScheme`, `ChromeOverlays`, `ChromePlugin`, `CrowbarFont`, `CrowbarFontModifier`, `CrowbarHeadingModifier`, `DrawerSwipe`, `FormAccessory`, `Haptics`, `HostedPage`, `PageChannel`, `SecondaryPage`, `ViewStyles`, `WeakMessageHandler`. Writing a placeholder would break M39-41; an exemption in `swift-rules.awk` for such files is the owner's call.
- Rust `global` on `src/frontend.rs`: Swift reaches the frontend through context-free C functions, so the installed frontend is a process-wide `OnceLock`. There is no context pointer to pass through `register_ios_plugin`.
