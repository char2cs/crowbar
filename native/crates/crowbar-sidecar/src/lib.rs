#![forbid(unsafe_code)]

//! `crowbar-sidecar` — `crowbar-app`'s own daemon lifecycle (spec §5.2, item
//! S0.3).
//!
//! **Not shared with `desktop/src-tauri`, on purpose.** The Tauri app is
//! scheduled for deletion and the Rust app is Crowbar's only surviving
//! frontend; a crate consumed by both would point the dependency backwards —
//! the codebase with a delete date owning an edge into the one that outlives
//! it, an edge someone then has to unpick. `desktop/src-tauri/src/sidecar/`
//! (1,042 lines: `mod.rs` 740, `supervisor.rs` 302) keeps working exactly as
//! it is and is not touched by this crate at all. What crosses over is the
//! **behaviour**, reproduced rather than transcribed — see the section below
//! for why that had to be true regardless of the sharing question. This
//! module owns:
//!
//! * [`home`] / [`socket`] — `CROWBAR_HOME` resolution mirroring the Go
//!   daemon's own order, and the byte-identical socket-path derivation
//!   (delegated to `crowbar_client::socket` for the arithmetic itself).
//! * [`binary`] — locating the `crowbar-api` executable, a question the
//!   Tauri module never had to answer because `tauri_plugin_shell` answered
//!   it.
//! * [`decision`] — the watchdog's pure policy: wedge detection, the
//!   suspension false-positive guard, and the restart budget.
//! * [`process`] — spawning the daemon with `std::process::Command`, pumping
//!   its stdout/stderr into the rotating daemon log, and sending it a named
//!   signal without `unsafe` (`sysinfo`, not `libc::kill`).
//! * [`watchdog`] — the deep-readiness probe loop built on those two.
//! * this module's [`ensure_daemon`] — the adopt-if-healthy entry point
//!   `crowbar-app` calls.
//!
//! # Why this reproduces behaviour rather than lifting code
//!
//! `desktop/src-tauri/src/sidecar/mod.rs` is Tauri-shaped throughout:
//! `AppHandle<R>`, `tauri_plugin_shell`'s `CommandChild`/`CommandEvent`,
//! `tauri::async_runtime::spawn`. `crowbar-app` is a GPUI binary with no
//! Tauri runtime and no tokio reactor (`crowbar-client`'s own doc comment
//! explains why: gpui runs its own executor), so even if this *were* a
//! shared crate, "lift, do not rewrite" could not have meant "move the
//! file" — the transport would still have had to become runtime-agnostic.
//! The daemon is spawned with plain `std::process::Command` and supervised
//! from plain OS threads instead. One concrete win fell out of that:
//! `std::process::Child` does not share a lock with a background wait
//! thread the way `tauri_plugin_shell`'s `shared_child`-backed
//! `CommandChild` does, so the elaborate "sign the health-reported pid,
//! never `CommandChild::pid()`" workaround the Tauri module carries to
//! dodge a live-observed deadlock (its `SidecarHandle::daemon_pid` doc
//! comment) has no equivalent hazard to work around here.
//!
//! Binary *location* is a genuinely new question ([`binary`]): the Tauri
//! module never had to answer it because `tauri_plugin_shell::Shell::sidecar`
//! answered it via Tauri's own packaged-resource resolution. Everything else
//! — socket-path resolution, the 104-byte `sun_path` cap, daemon-log
//! rotation, the watchdog's wedge detection and restart budget,
//! SIGTERM/SIGQUIT→SIGKILL shutdown — is the same policy, reproduced rather
//! than shared, running over a different transport. One behaviour the Tauri
//! module does not have at all: adopt-if-healthy (see [`ensure_daemon`]),
//! because the Tauri app always owns its daemon and never needs to borrow
//! one.

pub mod binary;
pub mod decision;
pub mod home;
pub mod process;
pub mod socket;
pub mod watchdog;

mod ensure;

pub use ensure::{EnsureError, Handle, Options, ensure_daemon};
