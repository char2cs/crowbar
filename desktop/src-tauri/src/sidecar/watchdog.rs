//! The watchdog: deep readiness probes of the running daemon, and the
//! goroutine dump plus restart when it is genuinely wedged.

use std::io::Write;
use std::path::PathBuf;
use std::sync::atomic::Ordering;
use std::time::Duration;

use http_body_util::BodyExt;
use tauri::{AppHandle, Emitter, Manager, Runtime};

use super::health::probe_ready;
use super::stop::{signal_then_kill, take_child};
use super::{epoch_secs, http_get, open_daemon_log, supervisor, SidecarHandle};

/// How often the watchdog probes the daemon's deep readiness path.
const WATCHDOG_INTERVAL: Duration = Duration::from_secs(10);
/// Per-probe budget; the deep path answers in microseconds when healthy.
const PROBE_TIMEOUT: Duration = Duration::from_secs(5);
/// A final, more generous probe budget after the failure threshold trips,
/// giving a daemon that just resumed from an OS suspension time to answer
/// before the watchdog commits to a restart.
const SUSPEND_GRACE_PROBE: Duration = Duration::from_secs(8);
/// Consecutive failed probes before the watchdog declares a wedge.
pub(super) const PROBE_FAILURE_THRESHOLD: u32 = 3;
/// Grace between SIGQUIT (Go dumps goroutines to captured stderr) and SIGKILL.
const SIGQUIT_GRACE: Duration = Duration::from_secs(2);

/// Fetches a full goroutine dump from the daemon's pprof surface, so a wedge
/// leaves behind the stacks that explain which lock everything was stuck on.
async fn capture_goroutine_dump(socket: &PathBuf) -> Option<String> {
    let resp = http_get(socket, "/debug/pprof/goroutine?debug=2")
        .await
        .ok()?;
    if !resp.status().is_success() {
        return None;
    }
    let body = resp.into_body().collect().await.ok()?.to_bytes();
    Some(String::from_utf8_lossy(&body).into_owned())
}

/// Watches the daemon and recovers it from wedges: after
/// PROBE_FAILURE_THRESHOLD consecutive failed deep probes it appends a
/// goroutine dump to the daemon log, SIGQUITs the daemon (the Go runtime
/// prints all stacks to the captured stderr and exits), and escalates to
/// SIGKILL; the output pump's Terminated handler then respawns within budget.
///
/// Spawned once per app run — respawned daemons are picked up automatically
/// because the probe reads the socket path from managed state on every tick.
pub fn start_watchdog<R: Runtime>(app: AppHandle<R>) {
    tauri::async_runtime::spawn(async move {
        let mut tracker = supervisor::FailureTracker::new(PROBE_FAILURE_THRESHOLD);
        let mut prev_cycle = std::time::SystemTime::now();
        loop {
            tokio::time::sleep(WATCHDOG_INTERVAL).await;

            // Wall-clock elapsed since the previous cycle. SystemTime — unlike a
            // monotonic Instant — keeps advancing through a system sleep, so an
            // OS suspension (App Nap / sleep) that froze this whole app surfaces
            // here as a gap far larger than the nominal cycle.
            let now = std::time::SystemTime::now();
            let cycle = now.duration_since(prev_cycle).unwrap_or(Duration::ZERO);
            prev_cycle = now;

            let state = app.state::<SidecarHandle>();
            if state.shutting_down.load(Ordering::SeqCst) {
                return;
            }
            // No child: daemon is mid-respawn or gave up its budget; nothing
            // to probe (and nothing the watchdog could kill anyway).
            if state.child.lock().unwrap().is_none() {
                tracker.reset();
                continue;
            }
            let Some(socket) = state.socket_path() else {
                continue;
            };

            // The app was suspended across this cycle (App Nap / system sleep):
            // the daemon was frozen alongside us, not wedged, and answers again
            // the instant it resumes. Counting this cycle's probe failure would
            // SIGQUIT a healthy daemon on wake — the exact false positive behind
            // the "backend closed itself while idle" reports. Discard the cycle.
            if supervisor::cycle_was_suspended(cycle, WATCHDOG_INTERVAL, PROBE_TIMEOUT) {
                tracker.reset();
                continue;
            }

            let outcome = tokio::time::timeout(PROBE_TIMEOUT, probe_ready(&socket))
                .await
                .unwrap_or(supervisor::Probe::Unserved);

            // We could not dial the daemon because *we* have no descriptors left.
            // That is our bug, not the daemon's, and no restart of it can hand one
            // back — the next probe would fail identically until the app is
            // restarted. Killing a backend on this evidence is how a healthy daemon
            // gets destroyed mid-session, so refuse to, and say so loudly.
            if outcome == supervisor::Probe::LocalDescriptorExhaustion {
                log::error!(
                    "the app is out of file descriptors and cannot dial the daemon. \
                     NOT restarting it — the daemon is not what is broken here."
                );
                tracker.reset();
                continue;
            }
            if !tracker.observe_probe(outcome) {
                continue;
            }

            // Three consecutive failed probes. Before the irreversible
            // SIGQUIT+SIGKILL, rule out the false positive that dominates in
            // practice: an OS-suspended daemon. macOS task-suspends an idle,
            // backgrounded helper; while suspended it fails every probe because
            // it is not executing, then answers the instant it resumes. Killing
            // it is the "backend closed itself while idle" bug. A genuinely
            // wedged daemon is running (state R/S) yet stuck, and answers
            // neither the probe nor a longer grace probe.
            let daemon_suspended = match state.daemon_pid() {
                Some(pid) => process_is_suspended(pid).await,
                None => false,
            };
            // A last, more generous probe. It exonerates the daemon two ways: it
            // answered after all (it had merely resumed, or was transiently slow),
            // or we could not even dial it because we are out of descriptors — in
            // which case the silence was never the daemon's to explain.
            let exonerated = if daemon_suspended {
                false
            } else {
                !supervisor::probe_indicts_daemon(
                    tokio::time::timeout(SUSPEND_GRACE_PROBE, probe_ready(&socket))
                        .await
                        .unwrap_or(supervisor::Probe::Unserved),
                )
            };
            if !supervisor::should_kill_wedged(daemon_suspended, exonerated) {
                log::warn!(
                    "crowbar daemon failed {PROBE_FAILURE_THRESHOLD} probes but is {} \
                     — not a wedge; leaving it to recover",
                    if daemon_suspended {
                        "OS-suspended"
                    } else {
                        "answering again"
                    }
                );
                tracker.reset();
                continue;
            }

            log::error!(
                "crowbar daemon failed {PROBE_FAILURE_THRESHOLD} consecutive readiness \
                 probes; capturing goroutine dump and restarting it"
            );
            let _ = app.emit("daemon:unhealthy", ());

            if let Some(dump) =
                tokio::time::timeout(Duration::from_secs(10), capture_goroutine_dump(&socket))
                    .await
                    .ok()
                    .flatten()
            {
                if let Some(mut f) = open_daemon_log() {
                    let _ = writeln!(
                        f,
                        "=== watchdog goroutine dump (epoch {}) ===\n{dump}\n=== end dump ===",
                        epoch_secs()
                    );
                }
            }

            kill_wedged(&app).await;
            tracker.reset();
        }
    });
}

/// Reports whether the process `pid` is OS-suspended (macOS task_suspend or a
/// job-control SIGSTOP) rather than running. `ps -o stat` reports a leading 'T'
/// for a stopped/suspended process and 'S'/'R' for a running one. On any error
/// we report false: better to let a genuine wedge be killed than to suppress a
/// restart because the process state could not be read.
async fn process_is_suspended(pid: i32) -> bool {
    let output = tokio::process::Command::new("ps")
        .args(["-o", "stat=", "-p", &pid.to_string()])
        .output()
        .await;
    match output {
        Ok(out) => String::from_utf8_lossy(&out.stdout)
            .trim_start()
            .starts_with('T'),
        Err(_) => false,
    }
}

/// SIGQUIT then SIGKILL for a daemon that stopped answering. SIGQUIT is not
/// caught by the daemon (it handles only SIGINT/SIGTERM), so the Go runtime's
/// default handler prints every goroutine stack to stderr — captured by the
/// output pump — and exits; SIGKILL covers a runtime too wedged even for that.
///
/// Signals go to the health-reported pid via libc; the CommandChild is taken
/// and dropped without calling into it — pid()/kill() lock the shared_child
/// mutex the shell plugin's wait thread holds for the child's lifetime, which
/// deadlocked this exact path in the 2026-07-04 live wedge drill.
async fn kill_wedged<R: Runtime>(app: &AppHandle<R>) {
    let Some((child, pid)) = take_child(app.state::<SidecarHandle>().inner()) else {
        return;
    };
    #[cfg(unix)]
    {
        let _ = tokio::task::spawn_blocking(move || {
            signal_then_kill(pid, libc::SIGQUIT, SIGQUIT_GRACE);
            drop(child);
        })
        .await;
    }
    #[cfg(not(unix))]
    {
        let _ = pid;
        let _ = child.kill();
    }
}
