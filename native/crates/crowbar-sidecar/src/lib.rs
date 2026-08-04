#![forbid(unsafe_code)]

//! `crowbar-sidecar` — daemon lifecycle, shared by `crowbar-app` and
//! `desktop/src-tauri` (spec §5.2, item S0.3).
//!
//! Lifted from `desktop/src-tauri/src/sidecar/` (1,042 lines: `mod.rs` 740,
//! `supervisor.rs` 302), which is now deleted — this crate is the one
//! implementation both binaries depend on, not two. It owns:
//!
//! * [`home`] / [`socket`] — `CROWBAR_HOME` resolution mirroring the Go
//!   daemon's own order, and the byte-identical socket-path derivation
//!   (delegated to `crowbar_client::socket` for the arithmetic itself).
//! * [`binary`] — locating the `crowbar-api` executable, a question the
//!   original never had to answer because `tauri_plugin_shell` answered it.
//! * [`decision`] — the watchdog's pure policy: wedge detection, the
//!   suspension false-positive guard, and the restart budget.
//! * [`process`] — spawning the daemon with `std::process::Command`, pumping
//!   its stdout/stderr into the rotating daemon log, and sending it a named
//!   signal without `unsafe` (`sysinfo`, not `libc::kill`).
//! * [`watchdog`] — the deep-readiness probe loop built on those two.
//! * this module's [`ensure_daemon`] — the adopt-if-healthy entry point every
//!   consumer actually calls.
//!
//! # Why this crate exists, and why it changed shape getting here
//!
//! The original was Tauri-shaped throughout: `AppHandle<R>`, `tauri_plugin_shell`'s
//! `CommandChild`/`CommandEvent`, `tauri::async_runtime::spawn`. `crowbar-app` is a
//! GPUI binary with no Tauri runtime and no tokio reactor (`crowbar-client`'s own
//! doc comment explains why: gpui runs its own executor), so "lift, do not
//! rewrite" could not mean "move the file" here — the transport had to become
//! runtime-agnostic first. The daemon is spawned with plain `std::process::Command`
//! and supervised from plain OS threads, which both a Tauri app and a GPUI app can
//! call into identically. One concrete win fell out of that: `std::process::Child`
//! does not share a lock with a background wait thread the way
//! `tauri_plugin_shell`'s `shared_child`-backed `CommandChild` does, so the
//! elaborate "sign the health-reported pid, never `CommandChild::pid()`" workaround
//! the original carried to dodge a live-observed deadlock (`mod.rs`'s
//! `SidecarHandle::daemon_pid` doc comment) has no equivalent hazard to work around
//! here.
//!
//! Binary *location* is a genuinely new question ([`binary`]): the original never
//! had to answer it because `tauri_plugin_shell::Shell::sidecar` answered it via
//! Tauri's own packaged-resource resolution. Everything else — socket-path
//! resolution, the 104-byte `sun_path` cap, daemon-log rotation, the watchdog's
//! wedge detection and restart budget, SIGTERM/SIGQUIT→SIGKILL shutdown — is the
//! same policy, unchanged, running over a different transport.

pub mod binary;
pub mod decision;
pub mod home;
pub mod process;
pub mod socket;
pub mod watchdog;

mod ensure;

pub use ensure::{EnsureError, Handle, Options, ensure_daemon};
