# webview

## Owns

Creating, placing and driving the native web views embedded as children of a GPUI window: the host
and its view handles, the `Platform` trait a backend implements, the GTK (Linux) and AppKit (macOS)
backends, and an in-memory mock backend for tests.

## Public interface

- `Host` and `Handle`: create a view in a parent window, then place, show, hide,
  navigate, evaluate script in and destroy it; `HostConfig`.
- What a view is: `Config` (with `IpcHandler`), `Id`, `Kind`, `Bounds`,
  `Color`, `Visibility`, `ParentWindow`, `Snapshot`, `PageEvent` and `PageHandler`.
- The `crowbar://` scheme: `ProtocolHandler`, `ProtocolResponder`, `ProtocolRequest`,
  `ProtocolResponse`.
- The backend port: `Platform` and its capabilities `ViewLifecycle`, `ViewLayout`, `PageControl`,
  `ViewFocus`, `ViewCapture`, `BrowsingData`.
- `Error`, `Result`; with feature `mock`: `MockPlatform` and `Call`.

## Depends on

- `protocol`: ids and frames of the pages a view shows.
- `platform`: the native parent-window handles (`XlibParent`, `AppkitParent`), the crate's only
  `unsafe`.
- External: wry and raw-window-handle; gtk and webkit2gtk on Linux, objc2-foundation on macOS.
