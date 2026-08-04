//! Spawning the daemon process itself, and everything that touches its OS
//! handle: stdout/stderr capture into the rotating daemon log, and sending it
//! a signal.
//!
//! Built on `std::process::Command` rather than `tauri_plugin_shell`'s
//! `shared_child`-backed wrapper — see the crate root doc comment for why —
//! and on `sysinfo` rather than `libc::kill` for sending a *named* signal
//! (`std::process::Child::kill` only ever sends SIGKILL) without `unsafe`.
//!
//! A spawned process's pid ([`Spawned::pid`]) is available the instant
//! `spawn` returns — `std::process::Child::id()` is a plain stored field, not
//! a value only known once a background wait thread reports it. That removes
//! a whole workaround the original carried: `desktop/src-tauri/src/sidecar
//! /mod.rs`'s `SidecarHandle::daemon_pid` doc comment explains why it had to
//! sign every kill with the daemon's *self-reported* pid from `/v0/health`
//! rather than `CommandChild::pid()` — that call locks a mutex the shell
//! plugin's wait thread holds for the child's entire lifetime, and deadlocked
//! live on 2026-07-04. `std::process::Child` shares no such lock.

use std::io::{BufRead, Read, Write};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::sync::atomic::{AtomicU32, Ordering};

use sysinfo::{ProcessesToUpdate, System};

pub use sysinfo::Signal;

use crate::decision;

/// Cap per daemon-log generation; one previous generation is kept as `.1`.
pub const DAEMON_LOG_MAX_LEN: u64 = 4 * 1024 * 1024;

/// How many captured output lines pass between rotation checks. The Go daemon
/// logs every request to stderr, so one long-lived daemon can outgrow the cap
/// between spawns; this is the periodic re-check for that case, distinct from
/// the check `open` already does once at spawn time.
const ROTATE_CHECK_EVERY_N_LINES: u32 = 5000;

/// What to run and how. Resolved entirely by the caller — this module does
/// not decide where the binary lives ([`crate::binary`]) or what home it
/// should use ([`crate::home`]); it only launches and supervises the OS
/// process it is given.
#[derive(Debug, Clone)]
pub struct SpawnConfig {
    pub program: PathBuf,
    pub args: Vec<String>,
    pub env: Vec<(String, String)>,
}

/// The child failed to start at all — the program was not found, was not
/// executable, or similar. A daemon that starts and then exits immediately is
/// not this: that is a normal [`Spawned`] whose [`Spawned::wait`] returns
/// quickly.
#[derive(Debug)]
pub struct SpawnError(std::io::Error);

impl std::fmt::Display for SpawnError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "could not start the daemon process: {}", self.0)
    }
}

impl std::error::Error for SpawnError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        Some(&self.0)
    }
}

/// A file the daemon's stdout/stderr are captured into, with periodic
/// rotation so the log never grows unbounded — the only place a Go panic's
/// stack trace survives in a packaged app, since the child's pipes are
/// otherwise connected to nothing.
///
/// `None` inside the mutex means opening (or reopening after rotation) the
/// file failed; every write against it is then a silent no-op rather than a
/// panic; a supervisor must survive a read-only disk, not crash on one.
#[derive(Debug)]
pub struct LogSink {
    path: PathBuf,
    file: Mutex<Option<std::fs::File>>,
    lines_since_rotate_check: AtomicU32,
}

impl LogSink {
    /// Opens (creating the parent directory and rotating an over-cap file
    /// first) the log at `path`.
    #[must_use]
    pub fn open(path: PathBuf) -> Self {
        let file = Mutex::new(open_for_append(&path));
        Self {
            path,
            file,
            lines_since_rotate_check: AtomicU32::new(0),
        }
    }

    /// The file this sink writes to — not necessarily open right now (a
    /// previous rotation or permission failure can leave it closed; see the
    /// struct doc comment), but always the path a fresh [`LogSink::open`]
    /// against the same daemon home would reopen.
    #[must_use]
    pub fn path(&self) -> &std::path::Path {
        &self.path
    }

    /// Appends one line, best-effort: a log write that fails must never take
    /// down the supervisor that is trying to record why the daemon did.
    pub fn write_line(&self, prefix: &str, text: &str) {
        self.maybe_rotate();
        if let Ok(mut guard) = self.file.lock()
            && let Some(file) = guard.as_mut()
        {
            let _ = writeln!(file, "{prefix} {}", text.trim_end_matches(['\r', '\n']));
        }
    }

    /// Every [`ROTATE_CHECK_EVERY_N_LINES`] lines, re-checks the file's size
    /// and reopens it — unconditionally on a successful check, matching the
    /// original's own cadence, since re-opening an already-fresh file is
    /// cheap and a rotated one otherwise keeps writing into the renamed `.1`.
    fn maybe_rotate(&self) {
        let previous = self
            .lines_since_rotate_check
            .fetch_add(1, Ordering::Relaxed);
        if previous + 1 < ROTATE_CHECK_EVERY_N_LINES {
            return;
        }
        self.lines_since_rotate_check.store(0, Ordering::Relaxed);
        if decision::rotate_if_needed(&self.path, DAEMON_LOG_MAX_LEN).is_ok()
            && let Ok(mut guard) = self.file.lock()
        {
            *guard = open_for_append(&self.path);
        }
    }
}

fn open_for_append(path: &std::path::Path) -> Option<std::fs::File> {
    if let Some(dir) = path.parent() {
        let _ = std::fs::create_dir_all(dir);
    }
    let _ = decision::rotate_if_needed(path, DAEMON_LOG_MAX_LEN);
    std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
        .ok()
}

fn epoch_secs() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_or(0, |d| d.as_secs())
}

/// A running child: its pid (known immediately, see this module's doc
/// comment) and the OS handle [`Spawned::wait`] reaps.
#[derive(Debug)]
pub struct Spawned {
    pid: u32,
    child: Child,
    stdout_pump: Option<std::thread::JoinHandle<()>>,
    stderr_pump: Option<std::thread::JoinHandle<()>>,
}

/// How the child ended, for the caller's log line and respawn decision.
#[derive(Debug)]
pub struct Exit {
    pub code: Option<i32>,
    #[cfg(unix)]
    pub signal: Option<i32>,
}

/// Starts `cfg.program`, capturing its stdout/stderr into `log` on two
/// background threads (one per stream — each is a plain blocking read loop,
/// nothing here needs to multiplex them on one thread).
///
/// # Errors
///
/// [`SpawnError`] if the OS could not start the process at all.
pub fn spawn(cfg: &SpawnConfig, log: &std::sync::Arc<LogSink>) -> Result<Spawned, SpawnError> {
    let mut command = Command::new(&cfg.program);
    command.args(&cfg.args);
    for (key, value) in &cfg.env {
        command.env(key, value);
    }
    command.stdout(Stdio::piped()).stderr(Stdio::piped());

    let mut child = command.spawn().map_err(SpawnError)?;
    let pid = child.id();

    log.write_line(
        "===",
        &format!("daemon spawned (epoch {}, pid {pid})", epoch_secs()),
    );

    let stdout_pump = child.stdout.take().map(|stdout| {
        let log = std::sync::Arc::clone(log);
        std::thread::spawn(move || pump(stdout, &log, "[out]"))
    });
    let stderr_pump = child.stderr.take().map(|stderr| {
        let log = std::sync::Arc::clone(log);
        std::thread::spawn(move || pump(stderr, &log, "[err]"))
    });

    Ok(Spawned {
        pid,
        child,
        stdout_pump,
        stderr_pump,
    })
}

/// Reads `stream` line by line — lossily, like the original's
/// `String::from_utf8_lossy` on each captured chunk, so one invalid byte
/// sequence in the daemon's own logging cannot truncate everything after it —
/// writing each line to `log` with `prefix`. Returns when the stream hits EOF,
/// which happens when the child's own fd closes: at process exit, or (for a
/// child that inherited no other copy of the pipe) immediately if it never
/// wrote anything before exiting.
fn pump(stream: impl Read, log: &LogSink, prefix: &str) {
    let mut reader = std::io::BufReader::new(stream);
    let mut buf = Vec::new();
    loop {
        buf.clear();
        match reader.read_until(b'\n', &mut buf) {
            Ok(0) | Err(_) => return,
            Ok(_) => log.write_line(prefix, &String::from_utf8_lossy(&buf)),
        }
    }
}

impl Spawned {
    /// The OS pid, valid for exactly as long as this process has not yet
    /// reaped it via [`Spawned::wait`] — POSIX will not recycle a pid still
    /// held by an un-reaped child of a live parent, so every [`signal`] call
    /// made with it before `wait` returns is guaranteed to reach this process
    /// and never a coincidentally-reused one.
    #[must_use]
    pub fn pid(&self) -> u32 {
        self.pid
    }

    /// Blocks until the child exits, reaping it, then joins the stdout/stderr
    /// pump threads so every line the child wrote before exiting is durably in
    /// the log before this returns — a caller that logs "daemon terminated"
    /// right after this call is guaranteed not to race the last of the
    /// daemon's own output.
    ///
    /// The one call in this module that blocks — on two real OS signals, a
    /// `waitpid` and then two thread joins, never a poll — so a caller that
    /// wants "wait for exit, with a timeout" pairs this with its own thread
    /// and channel rather than this module growing one itself.
    ///
    /// A pipe's write end is closed by the kernel as part of the child's own
    /// exit, strictly before `waitpid` can reap it — so by the time
    /// `child.wait()` below returns, both pumps' next read already sees EOF,
    /// and the joins that follow return promptly rather than blocking further.
    #[must_use]
    pub fn wait(mut self) -> Exit {
        let status = self.child.wait();
        if let Some(handle) = self.stdout_pump.take() {
            let _ = handle.join();
        }
        if let Some(handle) = self.stderr_pump.take() {
            let _ = handle.join();
        }
        match status {
            Ok(status) => Exit {
                code: status.code(),
                #[cfg(unix)]
                signal: {
                    use std::os::unix::process::ExitStatusExt as _;
                    status.signal()
                },
            },
            Err(_) => Exit {
                code: None,
                #[cfg(unix)]
                signal: None,
            },
        }
    }
}

/// Sends `signal` to `pid` without `unsafe`. Returns `false` both when the
/// process no longer exists (already gone — not an error, the common
/// "graceful shutdown already won the race" case) and when this platform does
/// not support the requested signal.
#[must_use]
pub fn signal(pid: u32, signal: Signal) -> bool {
    let pid = sysinfo::Pid::from_u32(pid);
    let mut system = System::new();
    system.refresh_processes(ProcessesToUpdate::Some(&[pid]));
    system
        .process(pid)
        .and_then(|process| process.kill_with(signal))
        .unwrap_or(false)
}

/// Whether `pid` still names a live process.
#[must_use]
pub fn exists(pid: u32) -> bool {
    let pid = sysinfo::Pid::from_u32(pid);
    let mut system = System::new();
    system.refresh_processes(ProcessesToUpdate::Some(&[pid]));
    system.process(pid).is_some()
}

/// Whether `pid` is OS-suspended (macOS `task_suspend`, or a job-control
/// SIGSTOP) rather than running — `sysinfo`'s cross-platform replacement for
/// the original's `ps -o stat=` shell-out: `ProcessStatus::Stop` is exactly
/// the leading `T` that parse checked for. `false` on any process this
/// process cannot inspect, or that has already exited: better to let a
/// genuine wedge be killed than to suppress a restart because the process
/// state could not be read.
#[must_use]
pub fn is_suspended(pid: u32) -> bool {
    let pid = sysinfo::Pid::from_u32(pid);
    let mut system = System::new();
    system.refresh_processes(ProcessesToUpdate::Some(&[pid]));
    system
        .process(pid)
        .is_some_and(|process| process.status() == sysinfo::ProcessStatus::Stop)
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::{LogSink, Signal, SpawnConfig, exists, is_suspended, signal, spawn};

    fn temp_dir(label: &str) -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "crowbar-sidecar-process-test-{label}-{}-{:?}",
            std::process::id(),
            std::thread::current().id(),
        ));
        std::fs::create_dir_all(&dir).expect("temp dir");
        dir
    }

    /// A process that stays alive — blocked on `read` from its own stdin —
    /// until this test explicitly signals it. Used by every test below that
    /// needs a *known-alive* pid to signal: no sleep is needed to be sure it
    /// is still running, because nothing in the script can make it exit on
    /// its own.
    fn spawn_blocking_probe(dir: &std::path::Path) -> (super::Spawned, Arc<LogSink>) {
        let log = Arc::new(LogSink::open(dir.join("daemon.log")));
        let cfg = SpawnConfig {
            program: std::path::PathBuf::from("/bin/sh"),
            args: vec!["-c".to_owned(), "read _unused".to_owned()],
            env: vec![],
        };
        let spawned = spawn(&cfg, &Arc::clone(&log)).expect("/bin/sh always exists in CI");
        (spawned, log)
    }

    /// A process that prints to both streams and exits on its own — no kill
    /// needed. `Spawned::wait` joins the pump threads after reaping it, so by
    /// the time this returns every line is guaranteed durable in the log; the
    /// test that uses this reads the log with no sleep and no retry loop.
    fn spawn_quick_probe(dir: &std::path::Path) -> (super::Spawned, Arc<LogSink>) {
        let log = Arc::new(LogSink::open(dir.join("daemon.log")));
        let cfg = SpawnConfig {
            program: std::path::PathBuf::from("/bin/sh"),
            args: vec![
                "-c".to_owned(),
                "echo out-line; echo err-line 1>&2".to_owned(),
            ],
            env: vec![],
        };
        let spawned = spawn(&cfg, &Arc::clone(&log)).expect("/bin/sh always exists in CI");
        (spawned, log)
    }

    #[test]
    fn a_spawned_process_pid_is_known_immediately_and_exists() {
        let dir = temp_dir("pid");
        let (spawned, _log) = spawn_blocking_probe(&dir);
        let pid = spawned.pid();
        assert!(pid > 0);
        assert!(exists(pid));

        let _ = signal(pid, Signal::Kill);
        // Blocks on waitpid, which only returns once the kernel has reaped
        // the zombie and removed its process-table entry — so `exists` right
        // after this needs no delay of its own to be accurate.
        let exit = spawned.wait();
        assert!(
            !exists(pid),
            "a killed, reaped process must not still exist"
        );
        assert_ne!(exit.code, Some(0), "SIGKILL is not a clean exit");

        std::fs::remove_dir_all(&dir).ok();
    }

    /// The mutation this test exists to catch: a `signal` that silently did
    /// nothing (e.g. targeted the wrong pid, or dropped the call) would leave
    /// the process alive, and `wait()` would then block forever — the test
    /// itself, not a sleep, is the timeout-free proof that the signal landed.
    #[test]
    fn signal_kill_actually_terminates_the_process() {
        let dir = temp_dir("kill");
        let (spawned, _log) = spawn_blocking_probe(&dir);
        let pid = spawned.pid();

        let sent = signal(pid, Signal::Kill);
        assert!(sent, "SIGKILL is supported on every platform this runs on");

        // Blocks on waitpid; no sleep, no poll — this returning at all is the
        // proof the signal was delivered.
        let exit = spawned.wait();
        assert!(exit.code.is_none() || exit.code != Some(0));

        std::fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn signal_on_an_already_gone_pid_reports_false_not_a_panic() {
        let dir = temp_dir("gone");
        let (spawned, _log) = spawn_blocking_probe(&dir);
        let pid = spawned.pid();
        let _ = signal(pid, Signal::Kill);
        // `wait()`'s waitpid is the real signal being blocked on: it cannot
        // return before the kernel has fully reaped the process, so `exists`
        // and a second `signal` immediately after are checking reality, not a
        // best-effort snapshot.
        let _ = spawned.wait();

        assert!(!exists(pid));
        assert!(!signal(pid, Signal::Term), "cannot signal a reaped pid");

        std::fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn spawning_a_missing_program_is_a_spawn_error_not_a_panic() {
        let log = Arc::new(LogSink::open(temp_dir("missing").join("daemon.log")));
        let cfg = SpawnConfig {
            program: std::path::PathBuf::from("/this/does/not/exist/crowbar-api"),
            args: vec![],
            env: vec![],
        };
        let err = spawn(&cfg, &log).expect_err("the program does not exist");
        assert!(err.to_string().contains("could not start"));
        assert!(std::error::Error::source(&err).is_some());
    }

    #[test]
    fn stdout_and_stderr_are_both_captured_into_the_log() {
        let dir = temp_dir("capture");
        let (spawned, _log) = spawn_quick_probe(&dir);

        // No sleep: `wait()` blocks on the process exiting on its own, then
        // joins both pump threads — by the time it returns, both lines are
        // guaranteed to already be in the file.
        let _ = spawned.wait();

        let contents = std::fs::read_to_string(dir.join("daemon.log")).unwrap_or_default();
        assert!(contents.contains("[out] out-line"), "{contents}");
        assert!(contents.contains("[err] err-line"), "{contents}");

        std::fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn a_running_process_is_not_reported_suspended() {
        let dir = temp_dir("not-suspended");
        let (spawned, _log) = spawn_blocking_probe(&dir);
        let pid = spawned.pid();
        assert!(!is_suspended(pid));
        let _ = signal(pid, Signal::Kill);
        let _ = spawned.wait();
        std::fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn log_sink_rotates_a_file_already_over_the_cap_on_open() {
        let dir = temp_dir("rotate-on-open");
        let log_path = dir.join("daemon.log");
        let oversized = usize::try_from(super::DAEMON_LOG_MAX_LEN + 1).unwrap_or(usize::MAX);
        std::fs::write(&log_path, vec![b'x'; oversized]).unwrap();

        let _sink = LogSink::open(log_path.clone());
        assert!(
            dir.join("daemon.log.1").exists(),
            "an over-cap file must be rotated aside before the first append"
        );

        std::fs::remove_dir_all(&dir).ok();
    }
}
