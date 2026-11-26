# panehost

## Owns

The native web views that show panes: pane types and where their pages come from, the pool of web
view slots, each pane's lifecycle frames, the inbox of what pages post, and the authorization of
every `crowbar://` request a page makes.

## Public interface

- `PaneHost`: `new`, `sync` (with `LayoutSnapshot`, `VisiblePane`, `Keyboard`; returns
  `SyncReport`), `handle_ipc`, `send`, `inbox` (`HostEvent`, `IpcMessage`), `menu_asks`
  (`MenuAsk`) and browser panes (`BrowseAction`).
- `Config`, `Error`, `Result`.
- Pane types: `PaneRegistry`, `PaneType`, `PaneKind`, `PaneLifecycle`, `Multiplicity`.
- Pane pages: the `BundleSource` port, `BundleAsset`, `DirBundle`, `ExtensionBundles`,
  `PaneBundles`.
- The `crowbar://` router: `PaneProtocol` and the `PaneTables` it shares with the host.

## Depends on

- `browser`: browser pane URLs and pick scripts.
- `layout`: pane ids and the layout the host mirrors.
- `protocol`: the pane bridge's frames and origins.
- `webview`: the native web views.
