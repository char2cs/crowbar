//! The deep-readiness probe loop: detects a wedged daemon (alive, but not
//! serving) and restarts it, while telling that apart from an OS-suspended
//! process and from this process's own descriptor exhaustion.
//!
//! Ported from `desktop/src-tauri/src/sidecar/mod.rs`'s `start_watchdog` /
//! `probe_ready` / `capture_goroutine_dump` / `kill_wedged`, rebuilt on a
//! plain OS thread (see the crate root doc comment for why) and
//! `crowbar_client::transport::get` instead of a hand-rolled
//! `UnixStream`+hyper client. The policy itself — `/v0/projects` rather than
//! `/v0/health` as the deep probe, the suspension false-positive guard, the
//! grace probe, SIGQUIT before SIGKILL — is unchanged.
//!
//! **Only ever watches a daemon this process itself spawned.** An adopted
//! daemon is never probed for wedge purposes: doing so would risk this
//! process SIGQUIT/SIGKILLing a daemon another app owns and is relying on —
//! exactly the hazard adopt-if-healthy exists to avoid. [`crate::ensure`]
//! enforces this by only ever starting a watchdog on `Handle::Owned`.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::time::{Duration, SystemTime};

use crate::decision::{self, FailureTracker, Probe};
use crate::process::{self, LogSink, Signal};

/// How often the watchdog probes the daemon's deep readiness path.
pub const WATCHDOG_INTERVAL: Duration = Duration::from_secs(10);
/// Per-probe budget; the deep path answers in microseconds when healthy.
pub const PROBE_TIMEOUT: Duration = Duration::from_secs(5);
/// A final, more generous probe budget after the failure threshold trips,
/// giving a daemon that just resumed from an OS suspension time to answer
/// before the watchdog commits to a restart.
pub const SUSPEND_GRACE_PROBE: Duration = Duration::from_secs(8);
/// Consecutive failed probes before the watchdog declares a wedge.
pub const PROBE_FAILURE_THRESHOLD: u32 = 3;
/// Grace between SIGQUIT (Go dumps goroutines to captured stderr) and SIGKILL.
pub const SIGQUIT_GRACE: Duration = Duration::from_secs(2);

/// What the watchdog observed, for a caller that wants to surface it (log
/// line, UI toast, …) beyond the daemon log every event is already written
/// to. Optional: [`run`] takes `Option<&dyn Observer>`, and every consumer of
/// this crate so far is content with just the log.
pub trait Observer: Send + Sync {
    fn on_event(&self, event: Event);
}

/// One watchdog-visible event, named after the four the original emitted to
/// the webview (`daemon:unhealthy`, `daemon:terminated` — folded into
/// [`Event::Killed`] here since this module is the only source of an
/// in-loop kill — `daemon:dead`, `daemon:restarted` is the caller's own
/// business once it receives [`Event::Wedged`], not this module's).
#[derive(Debug, Clone)]
pub enum Event {
    /// The failure threshold tripped and the daemon was not exonerated.
    Wedged,
    /// The wedged daemon was SIGQUIT/SIGKILLed.
    Killed,
}

/// Everything one watchdog cycle needs to dial the daemon and act on what it
/// finds.
pub struct Watch {
    pub socket: PathBuf,
    /// The **current** generation's pid. An `AtomicU32` rather than a plain
    /// `u32` because the daemon this watches may have been respawned by
    /// [`crate::ensure`]'s supervisor between one cycle and the next, and a
    /// wedge-kill signalling a stale pid would hit either nothing or —
    /// worse, if that number were ever reused — an unrelated process.
    pub pid: Arc<AtomicU32>,
    pub log: Arc<LogSink>,
    pub observer: Option<Arc<dyn Observer>>,
    /// Checked at the top of every cycle; set this to stop the loop before it
    /// starts its next `WATCHDOG_INTERVAL` sleep.
    pub shutting_down: Arc<AtomicBool>,
}

/// Runs the watchdog loop until `watch.shutting_down` is observed true.
/// Blocking — call this from a dedicated thread (`std::thread::spawn`), never
/// from a caller's own critical path.
pub fn run(watch: &Watch) {
    let mut tracker = FailureTracker::new(PROBE_FAILURE_THRESHOLD);
    let mut prev_cycle = SystemTime::now();

    loop {
        std::thread::sleep(WATCHDOG_INTERVAL);

        if watch.shutting_down.load(Ordering::SeqCst) {
            return;
        }

        // Wall-clock elapsed since the previous cycle. SystemTime — unlike a
        // monotonic Instant — keeps advancing through a system sleep, so an
        // OS suspension (App Nap / sleep) that froze this whole process
        // surfaces here as a gap far larger than the nominal cycle.
        let now = SystemTime::now();
        let cycle = now.duration_since(prev_cycle).unwrap_or(Duration::ZERO);
        prev_cycle = now;

        if decision::cycle_was_suspended(cycle, WATCHDOG_INTERVAL, PROBE_TIMEOUT) {
            tracker.reset();
            continue;
        }

        let outcome = probe_ready(&watch.socket, PROBE_TIMEOUT);

        // This process ran out of descriptors, not the daemon. Killing a
        // backend on that evidence is how a healthy daemon gets destroyed
        // mid-session, so refuse to and say so loudly.
        if outcome == Probe::LocalDescriptorExhaustion {
            watch.log.write_line(
                "===",
                "watchdog: this process is out of file descriptors and cannot dial the daemon. \
                 NOT restarting it — the daemon is not what is broken here.",
            );
            tracker.reset();
            continue;
        }

        if !tracker.observe(outcome == Probe::Healthy) {
            continue;
        }

        let pid = watch.pid.load(Ordering::SeqCst);
        if pid == 0 {
            // Mid-respawn: no live child to probe or kill this cycle.
            tracker.reset();
            continue;
        }

        // Three consecutive failed probes. Before the irreversible
        // SIGQUIT+SIGKILL, rule out the false positive that dominates in
        // practice: an OS-suspended daemon.
        let daemon_suspended = process::is_suspended(pid);
        let exonerated = if daemon_suspended {
            false
        } else {
            !decision::probe_indicts_daemon(probe_ready(&watch.socket, SUSPEND_GRACE_PROBE))
        };

        if !decision::should_kill_wedged(daemon_suspended, exonerated) {
            watch.log.write_line(
                "===",
                &format!(
                    "watchdog: failed {PROBE_FAILURE_THRESHOLD} probes but is {} — not a wedge; \
                     leaving it to recover",
                    if daemon_suspended {
                        "OS-suspended"
                    } else {
                        "answering again"
                    }
                ),
            );
            tracker.reset();
            continue;
        }

        watch.log.write_line(
            "===",
            &format!(
                "watchdog: failed {PROBE_FAILURE_THRESHOLD} consecutive readiness probes; \
                 capturing goroutine dump and restarting it"
            ),
        );
        notify(watch, Event::Wedged);

        if let Some(dump) = capture_goroutine_dump(&watch.socket) {
            watch
                .log
                .write_line("=== watchdog goroutine dump ===", &dump);
            watch.log.write_line("===", "end dump");
        }

        kill_wedged(pid);
        notify(watch, Event::Killed);
        tracker.reset();
    }
}

fn notify(watch: &Watch, event: Event) {
    if let Some(observer) = &watch.observer {
        observer.on_event(event);
    }
}

/// Deep readiness probe. Unlike `/v0/health` — which answers from a static
/// handler and stays green while the daemon is wedged — `/v0/projects` goes
/// through the global view store, the exact resource every observed
/// production wedge pinned. A daemon that cannot answer this is not serving
/// users, whatever its liveness endpoint says.
///
/// Dialling costs a descriptor of *this* process, so the probe reports
/// whether it could dial at all: an exhausted app cannot reach even a
/// perfectly healthy daemon, and the watchdog must not read that as the
/// daemon's fault.
fn probe_ready(socket: &std::path::Path, timeout: Duration) -> Probe {
    match crowbar_client::get(socket, "/v0/projects", timeout) {
        Ok(response) if response.is_success() => Probe::Healthy,
        Err(err) if is_descriptor_exhaustion(err.as_reqwest_error()) => {
            Probe::LocalDescriptorExhaustion
        }
        Ok(_) | Err(_) => Probe::Unserved,
    }
}

/// Whether a dial failed because this process is out of file descriptors.
/// `reqwest` wraps the underlying connect error, so the cause chain has to be
/// walked down to the `io::Error` that actually carries the OS error code.
/// `std::io::ErrorKind` does not name `EMFILE`/`ENFILE` individually (both map
/// to the generic `Uncategorized`/`Other` kind), so the raw number — from
/// `libc`'s constants, not a hand-copied literal — is the only way to tell
/// them apart from every other I/O error. Reading a `libc` constant is not an
/// FFI call, so this needs no `unsafe`.
fn is_descriptor_exhaustion(err: &(dyn std::error::Error + 'static)) -> bool {
    let mut cause: Option<&(dyn std::error::Error + 'static)> = Some(err);
    while let Some(e) = cause {
        if let Some(io) = e.downcast_ref::<std::io::Error>()
            && matches!(io.raw_os_error(), Some(libc::EMFILE | libc::ENFILE))
        {
            return true;
        }
        cause = e.source();
    }
    false
}

/// Fetches a full goroutine dump from the daemon's pprof surface, so a wedge
/// leaves behind the stacks that explain which lock everything was stuck on.
fn capture_goroutine_dump(socket: &std::path::Path) -> Option<String> {
    let response = crowbar_client::get(
        socket,
        "/debug/pprof/goroutine?debug=2",
        Duration::from_secs(10),
    )
    .ok()?;
    if !response.is_success() {
        return None;
    }
    Some(String::from_utf8_lossy(&response.body).into_owned())
}

/// SIGQUIT then SIGKILL for a daemon that stopped answering. SIGQUIT is not
/// caught by the daemon (it handles only SIGINT/SIGTERM), so the Go
/// runtime's default handler prints every goroutine stack to stderr —
/// captured by the log pump — and exits; SIGKILL covers a runtime too
/// wedged even for that.
fn kill_wedged(pid: u32) {
    let _ = process::signal(pid, Signal::Quit);
    std::thread::sleep(SIGQUIT_GRACE);
    // No-op if the SIGQUIT dump already ended it.
    let _ = process::signal(pid, Signal::Kill);
}

#[cfg(test)]
mod tests {
    use super::is_descriptor_exhaustion;

    #[test]
    fn an_io_error_carrying_emfile_is_reported_as_descriptor_exhaustion() {
        let err = std::io::Error::from_raw_os_error(libc::EMFILE);
        assert!(is_descriptor_exhaustion(&err));
    }

    #[test]
    fn an_io_error_carrying_enfile_is_reported_as_descriptor_exhaustion() {
        let err = std::io::Error::from_raw_os_error(libc::ENFILE);
        assert!(is_descriptor_exhaustion(&err));
    }

    #[test]
    fn an_unrelated_io_error_is_not_descriptor_exhaustion() {
        let err = std::io::Error::from(std::io::ErrorKind::ConnectionRefused);
        assert!(!is_descriptor_exhaustion(&err));
    }

    #[test]
    fn a_non_io_error_is_not_descriptor_exhaustion() {
        #[derive(Debug)]
        struct Other;
        impl std::fmt::Display for Other {
            fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                f.write_str("other")
            }
        }
        impl std::error::Error for Other {}
        assert!(!is_descriptor_exhaustion(&Other));
    }
}
