# Import of the Tauri-era user data

## Facts

- The old desktop app was Tauri 2 with identifier `software.rabbyte.crowbar`; its web view origin was `tauri://localhost`.
- It kept settings in localStorage as one key per setting (`crowbar:settings:<name>`, a JSON value) and the layout in IndexedDB `crowbar` (store `workspace-layout`).
- The native shell serves panes from `crowbar://local` with its own web view data directory (`$CROWBAR_HOME/shell/webview/core`). WebKit and WKWebView isolate storage per origin and per data directory, so no page of the new app can read the old store, and a hidden page on the old origin cannot be loaded without shipping the old scheme.

## Decision

1. Settings: the shell reads the old localStorage database natively, once, on the first start that finds no `legacy-imported` marker next to `settings.json`.
   - Linux: `$XDG_DATA_HOME/software.rabbyte.crowbar/localstorage/*.localstorage` (WebKitGTK SQLite, table `ItemTable(key, value)`, value UTF-16LE JSON).
   - macOS: `~/Library/WebKit/software.rabbyte.crowbar/WebsiteData/Default/*/*/LocalStorage/localstorage.sqlite3` (same table).
   - Only keys the shell owns are copied, and only when `settings.json` has no value for them; the marker is written after the attempt, whatever it found.
2. Layout: not imported. The IndexedDB layout is a structured-clone blob inside a WebKit-internal store, and the daemon already owns the placement tree (migrations `ledger-layout`, `placement-tree`), so chats and their panes come back from the daemon. The migration view should say: "Your settings were imported; open tabs were not, they reopen from your chats."
3. Deprecated keys (`syncSystemTheme`, `autoThemeLight`, `autoThemeDark`) are dropped, not migrated: `themeMode: system` is what they expressed.

## Not reachable

- Pane-local keys that stay in the pane origin's localStorage (keymaps, host configs) cannot be imported without a pane-side page reading the old store; they restart from defaults.
