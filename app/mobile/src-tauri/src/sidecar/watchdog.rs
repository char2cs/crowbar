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
/// Consecutive failed probes before the watchdog considers a wedge: about a minute and a half
/// of silence, so a daemon slowed by a loaded machine has time to answer.
pub(super) const PROBE_FAILURE_THRESHOLD: u32 = 6;
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

/// Recovers the daemon from wedges: after PROBE_FAILURE_THRESHOLD failed deep probes it
/// dumps goroutines to the daemon log, SIGQUITs, then SIGKILLs; the pump respawns within
/// budget. Spawned once: each tick re-reads the socket path, so respawns are picked up.
pub fn start_watchdog<R: Runtime>(app: AppHandle<R>) {
    tauri::async_runtime::spawn(async move {
        let mut tracker = supervisor::FailureTracker::new(PROBE_FAILURE_THRESHOLD);
        // When the current run of failed probes began, and the daemon's CPU time then.
        let mut silent_since: Option<(std::time::Instant, Option<Duration>)> = None;
        let mut prev_cycle = std::time::SystemTime::now();
        loop {
            tokio::time::sleep(WATCHDOG_INTERVAL).await;

            // SystemTime, unlike a monotonic Instant, advances through a system sleep, so an App
            // Nap or sleep that froze the app shows up as a gap far larger than the nominal cycle.
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
                silent_since = None;
                continue;
            }
            let Some(socket) = state.socket_path() else {
                continue;
            };

            // Suspended across this cycle: the daemon was frozen with us, not wedged, and counting
            // this failure would SIGQUIT a healthy daemon on wake. Discard the cycle.
            if supervisor::cycle_was_suspended(cycle, WATCHDOG_INTERVAL, PROBE_TIMEOUT) {
                tracker.reset();
                silent_since = None;
                continue;
            }

            let outcome = tokio::time::timeout(PROBE_TIMEOUT, probe_ready(&socket))
                .await
                .unwrap_or(supervisor::Probe::Unserved);

            // We are out of descriptors: our fault, and no restart of the daemon can fix it. Refuse
            // to kill a backend on this evidence, and say so loudly.
            if outcome == supervisor::Probe::LocalDescriptorExhaustion {
                log::error!(
                    "the app is out of file descriptors and cannot dial the daemon. \
                     NOT restarting it — the daemon is not what is broken here."
                );
                tracker.reset();
                silent_since = None;
                continue;
            }
            if !supervisor::probe_indicts_daemon(outcome) {
                silent_since = None;
            } else if silent_since.is_none() {
                silent_since = Some((std::time::Instant::now(), daemon_cpu_time(&state).await));
            }
            if !tracker.observe_probe(outcome) {
                continue;
            }

            // Before the irreversible SIGQUIT+SIGKILL, rule out the common false positive: macOS
            // task-suspends an idle background helper, which then fails every probe until it
            // resumes. A wedged daemon is running (R/S) and answers neither probe.
            let daemon_suspended = match state.daemon_pid() {
                Some(pid) => process_is_suspended(pid).await,
                None => false,
            };
            // A last, more generous probe exonerates the daemon if it answers, or if we could not
            // dial it at all for want of descriptors.
            let exonerated = if daemon_suspended {
                false
            } else {
                !supervisor::probe_indicts_daemon(
                    tokio::time::timeout(SUSPEND_GRACE_PROBE, probe_ready(&socket))
                        .await
                        .unwrap_or(supervisor::Probe::Unserved),
                )
            };
            // A restart needs evidence of a wedge: alive, silent past the whole budget, and idle
            // meanwhile. A daemon that answers, or is merely slow and still working, is never
            // restarted: each restart drops every session.
            let (silent_at, cpu_before) = silent_since
                .take()
                .unwrap_or((std::time::Instant::now(), None));
            let working = supervisor::made_progress(
                cpu_before,
                daemon_cpu_time(&state).await,
                silent_at.elapsed(),
            );
            if !supervisor::should_kill_wedged(daemon_suspended, exonerated, working) {
                log::warn!(
                    "crowbar daemon failed {PROBE_FAILURE_THRESHOLD} probes but is {} \
                     — not a wedge; leaving it to recover",
                    if daemon_suspended {
                        "OS-suspended"
                    } else if exonerated {
                        "answering again"
                    } else {
                        "busy (it kept using CPU)"
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

/// The CPU time the daemon we spawned has used so far, if it can be read.
async fn daemon_cpu_time(state: &SidecarHandle) -> Option<Duration> {
    let pid = state.daemon_pid()?;
    #[cfg(target_os = "linux")]
    {
        let stat = tokio::fs::read_to_string(format!("/proc/{pid}/stat"))
            .await
            .ok()?;
        // SAFETY: sysconf only reads a configuration value.
        let ticks = unsafe { libc::sysconf(libc::_SC_CLK_TCK) };
        supervisor::parse_proc_stat_cpu(&stat, u64::try_from(ticks).ok()?)
    }
    #[cfg(not(target_os = "linux"))]
    {
        let out = tokio::process::Command::new("ps")
            .args(["-o", "time=", "-p", &pid.to_string()])
            .output()
            .await
            .ok()?;
        supervisor::parse_ps_cpu_time(&String::from_utf8_lossy(&out.stdout))
    }
}

/// Whether `pid` is OS-suspended (`ps -o stat` leads with 'T'). Any error reads as
/// running: better to kill a genuine wedge than suppress a restart on a failed read.
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

/// SIGQUIT (uncaught by the daemon, so Go dumps every goroutine to the captured stderr)
/// then SIGKILL. Signals the recorded pid via libc and never calls into the CommandChild:
/// its pid()/kill() lock the mutex the shell plugin's wait thread holds, and deadlock.
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
