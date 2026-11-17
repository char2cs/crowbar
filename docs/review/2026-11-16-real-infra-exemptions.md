# Real-infrastructure tests that need an owner decision

The rust-rules checker flags `real-infra` (tempdir, filesystem, sockets, processes) in tests. These
are the findings left in shell_data, daemon_supervisor, transport and the mobile shell after the
burndown. Each exists because the behaviour under test is the operating-system behaviour itself, or
because the only seam is a concrete adapter whose port has not been cut yet. None is baselined or
suppressed; the checker still reports them.

## Inherently socket, process or permission tests

| Path | Findings | Why it cannot use an in-memory fake |
|---|---|---|
| `app/crates/transport/src/local.rs` | 9 | Dials a unix socket that binds late; the retry contract is about connect errors the kernel raises. |
| `app/crates/transport/src/local_get.rs` | 10 | HTTP over a unix socket: framing, size limit and hang-up are what is tested. |
| `app/crates/transport/src/socket_trust.rs` | 4 | Owner uid of a real socket file; only the kernel can claim another user. |
| `app/crates/transport/src/host_registry.rs` | 3 | File modes (0600/0700) written by the registry. |
| `app/crates/transport/tests/transport.rs`, `remote.rs` | 6, 2 | End-to-end over a loopback TLS socket and a unix socket. |
| `app/crates/transport/tests/support/echo_daemon.rs`, `tls_host.rs` | 6, 4 | The servers those end-to-end tests talk to. |
| `app/desktop/crates/daemon_supervisor/src/spawn_daemon.rs`, `stale_daemon.rs`, `daemon_paths.rs`, `supervisor.rs`, `supervision.rs`, `watchdog.rs` | 5, 2, 3, 2, 2, 3 | Spawn a real child (`sleep`, a missing binary) or listen on a unix socket to prove kill, reap and wedge detection. |
| `app/desktop/crates/daemon_supervisor/src/locate_binary.rs` | 6 | Execute bits on real files. |
| `app/desktop/crates/daemon_supervisor/tests/daemon_supervisor.rs`, `tests/support/daemon.rs` | 3, 6 | Integration test with a real daemon process. |
| `app/mobile/src-tauri/src/api_proxy/probe.rs`, `api_proxy/remote.rs`, `ws_bridge/remote.rs`, `test_support/proxy_fakes.rs` | 1, 1, 4, 6 | Pinned-certificate TLS handshakes and websocket upgrades against a loopback server; the rustls verifier is the unit under test. |
| `app/desktop/crates/shell_data/src/adapters/disk_files.rs` | 1 | The disk adapter of the new `FileStore` port; its test proves the real atomic write. |
| `app/desktop/crates/shell_data/src/legacy_sqlite.rs` | 1 | Reads a real sqlite file of the old app. |
| `app/desktop/crates/shell_data/src/icon_fetch.rs` | 4 | Writes icons through `atomic_file` into a real directory; needs an injected writer to fake (not cut yet). |

## Port not yet cut (decision: cut it, or accept the exemption)

`daemon_supervisor`: `DaemonLog` opens its file through `adapters::LogWriter` directly, so
`log_writer.rs` (13), `daemon_log.rs` (7) and `output_pump.rs` (4) test through a real directory.
Cutting a `LogSink` port would let those be in-memory; the rotation and 0600 behaviour would then
stay in `log_writer.rs` only.

## Structural, not real-infra

`app/desktop/crates/shell_data/src/sidebar_view.rs` has 19 functions (limit 12) because it is the
single `sidebar::SidebarData` trait implementation. Splitting it needs the trait split in the
`sidebar` crate, which another agent owns.
