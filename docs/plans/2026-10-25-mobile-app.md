# Crowbar iOS app (v1)

iPhone-only Tauri 2 app, a thin client to remote daemons. The React UI is reused; the chrome is native SwiftUI (Liquid Glass, iOS 26 SDK). No Go changes.
Out of scope: iPad layout, push notifications, file explorer, local daemon, editor, browser pane, keybindings.

## Architecture

- `desktop/src-tauri` stays the one crate (`[lib]` is already `staticlib`/`cdylib`). `tauri ios init` adds `gen/apple`; `lib.rs` gets `#[cfg_attr(mobile, tauri::mobile_entry_point)]`.
- One webview, one JS context, one set of stores. The sidebar is not a second webview.
- Transport is the existing Rust `ws_bridge` dial with fingerprint pinning (`tls_pin.rs`), so WKWebView never opens the socket and App Transport Security does not apply. Tailscale only supplies reachability.
- Only remote hosts exist on iOS. The local daemon path (`sidecar`, the unix-socket `api_proxy`, `fdlimit`) is gated `#[cfg(desktop)]`; the host list never offers "local".
- Pairing is the desktop flow: `host:port` plus OTP, fingerprint confirmed once, token in the Keychain via `keyring` (`apple-native` already covers iOS). Any number of hosts; the switcher is the sidebar heading, as on desktop.

## Build gating (Rust)

Excluded from the iOS build: `window-vibrancy`, `tauri-plugin-macos-fps`, `macos-private-api`, `tauri-plugin-shell`, the embedded browser (`src/browser`), the diagnostics zip export if it shells out, and the sidecar. Move them to `[target.'cfg(not(any(target_os = "ios", target_os = "android")))'.dependencies]`. `capabilities/` splits: the default capability carries `shell:*` and stays desktop-only; a `mobile.json` carries `core:default`, `dialog`, and `log`.

## Screens

Everything below is the existing React feature, with a mobile layout selected by a viewport/platform store value. No component forks: the chat transcript, composer content, tree rows, terminal, and review components are the same ones.

| Screen | Content (web) | Chrome (native SwiftUI) |
|---|---|---|
| Chat | transcript + composer field | glass buttons: chat list (left); IDE shell, branch review (right) |
| Chat list | existing sidebar tree, full height | glass settings button, host heading, new thread |
| IDE shell | terminal, input bar, modifier row (Esc Tab Ctrl arrows) | back, title |
| Branch review | file list -> unified diff -> threads (GitHub mobile style) | back, title |
| Settings | native sheet | list: Appearance, Terminal, Agents, Git, haptics, diagnostics |

- The chat list opens by a left-to-right swipe or the top-left button. The drag is handled in JS (touch events, CSS transform on the content layer) so one layer owns gesture and animation. The glass buttons float above it, native.
- Long-press on a tree row reports `{ rect, actions }` to Swift over the plugin bridge and Swift presents a native `UIMenu`. `actions` come from the same descriptor the desktop context menu uses, so there is no second list. The on-row buttons are hidden on mobile.
- Composer: the input stays a web element (native text input would copy the draft and attachments into a second owner). Model pill, `+`, and send are native glass buttons positioned from the web layout rect.

## Swift plugin boundary

One Tauri iOS plugin (`desktop/src-tauri/ios-plugin`) exposes: `setChrome({ screen, title, buttons })`, `presentMenu({ rect, actions })`, `presentSettings()`, and an event channel back to JS for button taps. The web store owns all state; Swift renders what it is handed and emits intents. It never keeps its own copy.

## Verification gates

1. **Spike (first, throwaway branch commit not kept):** a Swift glass button over the Tauri webview, tapping into a JS event and back. Confirms the plugin model and that glass renders over WKWebView content.
2. Real-device run over Tailscale: pair, open a chat, stream a turn, open the shell, open a review.
3. Drag-to-open sidebar holds frame rate on device.
4. Check list for the simulator: `xcodebuild`, `tauri ios dev` on `aarch64-apple-ios-sim`.

## Open items to confirm while building

- `tauri-plugin-mcp-bridge` (debug only) on iOS: unknown whether it runs there; if not, gate it.
- Whether `web/` code reads desktop-only globals (`window.__TAURI__` window APIs, vibrancy, traffic-light insets) at import time; each needs a platform guard.
- Safe-area and keyboard insets feeding the composer layout.

## Rules

Each task starts with a failing test, extends the unit's existing test file under `web/src/__tests__/`, deletes what it makes unused, and keeps `knip`, `eslint`, and `tsc` clean. Components use `@/components/ui/*` and CSS tokens, kebab-case files, and phosphor icons. No file past 500 lines.
