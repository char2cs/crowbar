# Webview storage per Crowbar home

## Problem

The desktop webview's origin (`tauri://localhost`) is the same for every Crowbar
home. Its localStorage and its IndexedDB `crowbar` database therefore outlive a home
switch (backup restore, legacy-home migration, `CROWBAR_HOME` pointed elsewhere):

- the window layout (`workspace-layout` row `window`) restores views and editor
  tabs whose projects and workspaces exist only in the previous home. Every
  restored tab toasts "Failed to open file", and the old project's views stay
  in the layout, hidden;
- `crowbar.hosts` restores remote hosts that were paired from the previous home;
- `crowbar.activeProject`, `workspace:<id>:state`, composer and editor drafts
  carry ids that mean nothing to the new home.

## Owner

The desktop shell knows the home before any page script runs. The init script in
`desktop/src-tauri/src/init_scripts.rs` already seeds `window.__CROWBAR__`.

## Plan

1. The shell adds `home: <stable id>` to `__CROWBAR__`, where the id is a hash of
   the canonical `CROWBAR_HOME` path. The default home publishes no id.
2. `lib/local-storage.ts` and every zustand `persist` store resolve keys through one
   `homeKey(key)` helper. Without an id it returns `key` unchanged, so existing
   installs keep their state. With an id it returns `key@<id>`.
3. `lib/persistence/idb.ts` opens `crowbar` for the default home and
   `crowbar@<id>` otherwise.
4. Web builds served by a remote daemon are unaffected: a browser origin is
   already one per daemon.

## Tests

- **Shell:** the init script carries the id for a non-default home and none for
  the default home.
- **Web:** a value written under one home is not read under another. The default
  home reads the keys that existing installs already hold.
