# Real-infrastructure tests that cannot use an in-memory fake (UI crates)

Each entry needs an owner decision: keep as an exempted real-infra test, or approve a port for it.

## atomic_file, shell_diagnostics, terminal_session (terminal group)

- `app/crates/atomic_file/src/adapters/disk_write.rs` (6 findings): the crate is the filesystem adapter itself; its tests assert 0600 mode, rename-over and cleanup of the staged file, which only a real filesystem can show.
- `app/desktop/crates/shell_diagnostics/src/adapters/disk_files.rs`, `adapters/disk_log_store.rs`: one smoke test each for the disk adapters behind the `FileStore` and `LogStore` ports; all other tests use in-memory fakes.
- `app/desktop/crates/terminal_session/src/terminal_sessions.rs` (7 findings): the end-to-end session tests drive the real websocket client against a loopback `TcpListener` relay (handshake subprotocol, close and reattach behaviour). A fake would need a connector port around `tokio_tungstenite::client_async`; decision wanted whether that port is worth it.

## ui

- `app/desktop/crates/ui/src/assets.rs` (3 findings): the tests compare the embedded icon table with the files in `assets/icons` on disk (orphan and missing-weight check). The point of the test is the real directory, so an in-memory fake would test nothing; decision wanted whether to keep it as an exempted real-fs test or to generate the table at build time.

## sidebar, window_state, pane_host (adapter tests)

Each is the single test of a filesystem adapter; everything above the adapter uses an in-memory fake.

| File | Why it needs a real filesystem |
|------|-------------------------------|
| `app/desktop/crates/sidebar/src/adapters/disk_preferences.rs` | the adapter itself: a missing file reads as nothing, a written one comes back (atomic write) |
| `app/desktop/crates/window_state/src/adapters/disk_file.rs` | same, plus the nested directory is created on first write |
| `app/desktop/crates/pane_host/src/adapters/dir_bundle.rs` | serves files from a directory and refuses paths that leave it; the traversal guarantee is about real path resolution |
