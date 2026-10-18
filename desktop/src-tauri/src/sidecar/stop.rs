//! Ending the daemon on purpose: the app is quitting, so SIGTERM with a grace
//! before SIGKILL.

use std::sync::atomic::Ordering;
use std::time::{Duration, Instant};

use tauri_plugin_shell::process::CommandChild;

use super::SidecarHandle;

/// How long an intentional stop waits for SIGTERM's graceful shutdown (Container.Close
/// → Terminal.Shutdown → flush + persist) before resorting to SIGKILL.
const SIGTERM_GRACE: Duration = Duration::from_secs(3);

/// Stops the daemon for good — the app is ending (last window closed, or Cmd+Q):
/// SIGTERM, so it flushes and persists its terminals for the next launch, then
/// SIGKILL only if it is still running when the grace runs out. Blocks for at most
/// the grace; idempotent (the second ending finds no child).
///
/// Called from BOTH ways the app can end, because they are genuinely different code
/// paths in tao: the last window being destroyed, and `RunEvent::Exit` — which is
/// what Cmd+Q produces without closing windows individually.
pub fn shutdown(state: &SidecarHandle, reason: &str) {
    // Tell the supervisor this exit is intentional so neither the output pump nor
    // the watchdog respawns the daemon.
    state.shutting_down.store(true, Ordering::SeqCst);
    let Some((child, pid)) = take_child(state) else {
        log::debug!("daemon shutdown ({reason}): already stopped");
        return;
    };
    log::info!("stopping crowbar daemon ({reason}, pid {pid})");
    #[cfg(unix)]
    {
        signal_then_kill(pid, libc::SIGTERM, SIGTERM_GRACE);
        drop(child);
    }
    #[cfg(not(unix))]
    {
        let _ = pid;
        let _ = child.kill();
    }
}

/// Takes the running child and its pid out of the handle, so exactly one stop path
/// owns ending it and no later path can signal a pid that has since been recycled.
pub(super) fn take_child(state: &SidecarHandle) -> Option<(CommandChild, i32)> {
    let child = state.child.lock().unwrap().take()?;
    let pid = state.daemon_pid.swap(0, Ordering::SeqCst);
    Some((child, pid))
}

/// Sends `first` to `pid`, waits up to `grace` for the process to go, and SIGKILLs
/// it only if it has not. A pid of 0 (never recorded) signals nothing.
#[cfg(unix)]
pub(super) fn signal_then_kill(pid: i32, first: libc::c_int, grace: Duration) {
    if pid <= 0 {
        return;
    }
    let pid = pid as libc::pid_t;
    unsafe { libc::kill(pid, first) };
    let deadline = Instant::now() + grace;
    // Poll before the first sleep: a daemon that exits promptly is the normal case.
    loop {
        // kill(pid, 0) fails (ESRCH) once the process is gone and reaped.
        if unsafe { libc::kill(pid, 0) } != 0 {
            return;
        }
        if Instant::now() >= deadline {
            break;
        }
        std::thread::sleep(Duration::from_millis(50));
    }
    unsafe { libc::kill(pid, libc::SIGKILL) };
}

#[cfg(test)]
mod tests {
    #[cfg(unix)]
    mod signal_then_kill {
        use super::super::signal_then_kill;
        use std::process::Command;
        use std::time::{Duration, Instant};

        /// Spawn `script` and reap it on a thread, as the shell plugin's wait thread
        /// does for the real daemon — an unreaped zombie still answers kill(pid, 0).
        /// Callers hold the fd-test lock: spawning opens descriptors.
        fn spawn_reaped(script: &str) -> (i32, std::thread::JoinHandle<()>) {
            let mut child = Command::new("/bin/sh")
                .args(["-c", script])
                .spawn()
                .unwrap();
            let pid = child.id() as i32;
            let reaper = std::thread::spawn(move || {
                let _ = child.wait();
            });
            (pid, reaper)
        }

        #[test]
        fn a_process_that_honours_the_first_signal_is_not_waited_out() {
            let _serialised = crate::test_support::fd_tests_blocking();
            let (pid, reaper) = spawn_reaped("exec sleep 30");
            let started = Instant::now();
            signal_then_kill(pid, libc::SIGTERM, Duration::from_secs(10));
            reaper.join().unwrap();
            assert!(
                started.elapsed() < Duration::from_secs(5),
                "a daemon that exits on SIGTERM must not be held for the whole grace"
            );
        }

        #[test]
        fn a_process_that_ignores_the_first_signal_is_killed_after_the_grace() {
            let _serialised = crate::test_support::fd_tests_blocking();
            // Signal-ignoring: the shell traps TERM and waits on its child.
            let (pid, reaper) = spawn_reaped("trap '' TERM; while :; do sleep 1; done");
            std::thread::sleep(Duration::from_millis(100)); // let the trap install
            signal_then_kill(pid, libc::SIGTERM, Duration::from_millis(200));
            reaper.join().unwrap(); // returns only because SIGKILL ended it
            assert_ne!(unsafe { libc::kill(pid, 0) }, 0);
        }

        #[test]
        fn a_zero_pid_signals_nothing() {
            // kill(0, sig) would signal our whole process group.
            signal_then_kill(0, libc::SIGTERM, Duration::from_millis(1));
        }
    }
}
