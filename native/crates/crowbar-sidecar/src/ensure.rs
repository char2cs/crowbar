//! The crate's front door: [`ensure_daemon`].
//!
//! **Adopt-if-healthy, spawn only if not.** Both apps must be able to run
//! simultaneously against one `CROWBAR_HOME` — that is what makes the
//! side-by-side review instrument (`native/scripts/capture-pair.swift`,
//! S0.6) possible, and it is the double-spawn trap `native/QUEUE.md`'s
//! former item 0.4 section documents: `spawn()` used to be unconditional, so
//! a second app launched against an already-served socket would fail to
//! bind, sit in a budgeted respawn loop it could never win, and — this is
//! the part that made it dangerous rather than merely wasteful — an
//! adopting app that SIGTERMs on its own exit would kill the daemon out from
//! under whichever app actually started it.
//!
//! [`Handle`] is the guard against that last part: an [`Handle::Adopted`]
//! daemon is never signalled, on shutdown or on drop, full stop.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::time::{Duration, Instant};

use crate::decision::RestartBudget;
use crate::home::CrowbarHome;
use crate::process::{self, LogSink, Signal, SpawnConfig};
use crate::watchdog::{self, Watch};
use crate::{binary, home, socket};

/// Per-attempt budget for the startup health probe. A booting daemon answers
/// `/v0/health` immediately or not at all, so this only has to outlast a
/// loaded machine — never a slow request.
const HEALTH_PROBE_TIMEOUT: Duration = Duration::from_secs(3);
/// How many times [`ensure_daemon`] retries the startup probe against a
/// freshly spawned daemon before giving up.
const HEALTH_PROBE_ATTEMPTS: u32 = 30;
const HEALTH_PROBE_RETRY_DELAY: Duration = Duration::from_millis(200);
/// At most this many automatic respawns per window; beyond it the daemon is
/// declared dead until the next launch (guards against crash loops).
const RESTART_MAX: usize = 3;
const RESTART_WINDOW: Duration = Duration::from_mins(10);
/// Grace between an unexpected exit and a respawn attempt.
const RESPAWN_DELAY: Duration = Duration::from_millis(500);
/// Grace between SIGTERM and SIGKILL on an intentional shutdown.
const SIGTERM_GRACE: Duration = Duration::from_secs(3);

/// Everything [`ensure_daemon`] needs that this crate would otherwise have to
/// resolve from the environment itself — every field defaults to the real
/// resolution ([`crate::socket::socket_path`], [`crate::binary::locate`],
/// [`crate::home::resolve`]) so production callers can use
/// [`Options::default`], and tests can override exactly the parts a
/// filesystem- and environment-free test needs to control.
pub struct Options {
    pub socket: Option<PathBuf>,
    pub daemon_binary: Option<PathBuf>,
    pub home: Option<CrowbarHome>,
    /// Told about watchdog events on an owned daemon. Never consulted for an
    /// adopted one — see [`crate::watchdog`]'s doc comment for why.
    pub observer: Option<Arc<dyn watchdog::Observer>>,
    /// Whether to start the watchdog thread for a daemon this call spawns.
    /// Defaults to `true`; a test that only cares about the adopt/spawn
    /// decision sets this `false` so it does not also have to outlive a
    /// thread probing every [`watchdog::WATCHDOG_INTERVAL`].
    pub watchdog: bool,
    /// Per-attempt budget, attempt count, and delay between attempts for the
    /// startup/adopt-check health probe. Production defaults match the
    /// original's (3s / 30 attempts / 200ms — up to ~6s to give up on a
    /// freshly spawned daemon that never comes up); a test that spawns a
    /// deliberately non-answering stand-in tightens these so it is not stuck
    /// waiting out that whole budget for a negative result it already knows.
    pub health_probe_timeout: Duration,
    pub health_probe_attempts: u32,
    pub health_probe_retry_delay: Duration,
}

impl Default for Options {
    fn default() -> Self {
        Self {
            socket: None,
            daemon_binary: None,
            home: None,
            observer: None,
            watchdog: true,
            health_probe_timeout: HEALTH_PROBE_TIMEOUT,
            health_probe_attempts: HEALTH_PROBE_ATTEMPTS,
            health_probe_retry_delay: HEALTH_PROBE_RETRY_DELAY,
        }
    }
}

/// Why [`ensure_daemon`] could not produce a running daemon at all.
#[derive(Debug)]
pub enum EnsureError {
    /// No `crowbar-api` binary could be found (only reached when adoption was
    /// not possible — an already-healthy socket never needs one).
    BinaryNotFound(binary::NotFound),
    /// The OS could not start the daemon process.
    Spawn(process::SpawnError),
    /// The daemon was spawned but never answered `/v0/health` within the
    /// startup budget.
    NeverBecameHealthy,
}

impl std::fmt::Display for EnsureError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::BinaryNotFound(err) => write!(f, "{err}"),
            Self::Spawn(err) => write!(f, "{err}"),
            Self::NeverBecameHealthy => {
                f.write_str("the daemon was spawned but never answered /v0/health in time")
            }
        }
    }
}

impl std::error::Error for EnsureError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::BinaryNotFound(err) => Some(err),
            Self::Spawn(err) => Some(err),
            Self::NeverBecameHealthy => None,
        }
    }
}

/// A live daemon this call either adopted or spawned.
#[derive(Debug)]
pub enum Handle {
    /// This call spawned the daemon and therefore owns it: watched (unless
    /// [`Options::watchdog`] was `false`) and killed on
    /// [`Handle::shutdown`]/drop.
    Owned(Owned),
    /// The socket already answered healthy before this call touched
    /// anything. **Never signalled, on shutdown or on drop** — the whole
    /// point of adopt-if-healthy is that an app which did not start the
    /// daemon must not be able to end it out from under whoever did.
    Adopted { socket: PathBuf },
}

/// The state behind [`Handle::Owned`].
#[derive(Debug)]
pub struct Owned {
    socket: PathBuf,
    log: Arc<LogSink>,
    current_pid: Arc<AtomicU32>,
    shutting_down: Arc<AtomicBool>,
    shutdown_called: AtomicBool,
}

impl Handle {
    /// The socket the daemon is listening on, whichever way this handle came
    /// to hold it.
    #[must_use]
    pub fn socket_path(&self) -> &std::path::Path {
        match self {
            Self::Owned(owned) => &owned.socket,
            Self::Adopted { socket } => socket,
        }
    }

    /// Whether this call spawned the daemon (`true`) or adopted an
    /// already-healthy one (`false`).
    #[must_use]
    pub fn is_owned(&self) -> bool {
        matches!(self, Self::Owned(_))
    }

    /// The daemon's current OS pid, for an owned daemon — `None` for an
    /// adopted one (this handle never learns the other app's daemon's pid,
    /// and must not: that is exactly the information it would need to be
    /// able to signal it) and briefly `None` for an owned one mid-respawn.
    #[must_use]
    pub fn pid(&self) -> Option<u32> {
        match self {
            Self::Owned(owned) => match owned.current_pid.load(Ordering::SeqCst) {
                0 => None,
                pid => Some(pid),
            },
            Self::Adopted { .. } => None,
        }
    }

    /// Stops the daemon **iff this call started it**. Idempotent — safe to
    /// call more than once, and called automatically on drop if a caller
    /// never calls it explicitly.
    ///
    /// SIGTERM, then (after a daemon that does not exit within
    /// [`SIGTERM_GRACE`]) SIGKILL — matching the original's own comment on
    /// the equivalent path: "wait up to 3s for its graceful shutdown
    /// (Container.Close → Terminal.Shutdown → flush+persist), then SIGKILL."
    pub fn shutdown(&self, reason: &str) {
        let Self::Owned(owned) = self else {
            // Adopted: never signalled. This is not a missing case, it is
            // the guarantee the whole module exists for.
            return;
        };
        if owned.shutdown_called.swap(true, Ordering::SeqCst) {
            return;
        }
        owned.shutting_down.store(true, Ordering::SeqCst);

        let pid = owned.current_pid.load(Ordering::SeqCst);
        if pid == 0 {
            return;
        }
        owned
            .log
            .write_line("===", &format!("stopping daemon ({reason}, pid {pid})"));

        let _ = process::signal(pid, Signal::Term);
        let deadline = Instant::now() + SIGTERM_GRACE;
        while process::exists(pid) && Instant::now() < deadline {
            std::thread::sleep(Duration::from_millis(100));
        }
        // No-op (process not found) if the SIGTERM already ended it.
        let _ = process::signal(pid, Signal::Kill);
    }
}

impl Drop for Handle {
    fn drop(&mut self) {
        self.shutdown("handle dropped");
    }
}

/// Probes the socket first; spawns only if nothing answers. If a healthy
/// daemon is already listening, **adopts** it — does not spawn, and (see
/// [`Handle::shutdown`]) does not kill it later either.
///
/// # Errors
///
/// [`EnsureError`] when adoption was not possible (nothing answered) *and*
/// spawning also failed — no binary found, the OS could not start it, or it
/// never became healthy within the startup budget.
pub fn ensure_daemon(options: &Options) -> Result<Handle, EnsureError> {
    let socket = options.socket.clone().unwrap_or_else(socket::socket_path);

    // Adopt-if-healthy: this is the one line the behaviour lives in. Everyone
    // reading this function's diff should be able to point at it.
    if crowbar_client::fetch_health_with_timeout(&socket, options.health_probe_timeout).is_ok() {
        return Ok(Handle::Adopted { socket });
    }

    let daemon_binary = match &options.daemon_binary {
        Some(path) => path.clone(),
        None => binary::locate().map_err(EnsureError::BinaryNotFound)?,
    };
    let resolved_home = options.home.clone().or_else(home::resolve);
    let log = Arc::new(LogSink::open(
        resolved_home
            .as_ref()
            .map_or_else(home::daemon_log_path, |h| {
                h.path.join("logs").join("daemon.log")
            }),
    ));

    let cfg = spawn_config(&daemon_binary, &socket, resolved_home.as_ref());
    let spawned = process::spawn(&cfg, &log).map_err(EnsureError::Spawn)?;
    let current_pid = Arc::new(AtomicU32::new(spawned.pid()));

    if wait_for_health(
        &socket,
        options.health_probe_attempts,
        options.health_probe_timeout,
        options.health_probe_retry_delay,
    )
    .is_none()
    {
        // Never became healthy: this handle is not usable, so there is
        // nothing to hand back — stop the process we just started rather
        // than leaking it, then report why.
        let _ = process::signal(spawned.pid(), Signal::Kill);
        let exit = spawned.wait();
        log.write_line(
            "===",
            &format!(
                "daemon never became healthy; killed it (code={:?}, signal={:?})",
                exit.code, exit.signal
            ),
        );
        return Err(EnsureError::NeverBecameHealthy);
    }

    let shutting_down = Arc::new(AtomicBool::new(false));
    let observer = options.observer.clone();

    let supervise_log = Arc::clone(&log);
    let supervise_pid = Arc::clone(&current_pid);
    let supervise_shutting_down = Arc::clone(&shutting_down);
    let supervise_cfg = cfg.clone();
    let supervise_observer = observer.clone();
    std::thread::spawn(move || {
        supervise(
            spawned,
            &supervise_cfg,
            &supervise_log,
            &supervise_pid,
            &supervise_shutting_down,
            supervise_observer.as_deref(),
        );
    });

    if options.watchdog {
        let watch = Watch {
            socket: socket.clone(),
            pid: Arc::clone(&current_pid),
            log: Arc::clone(&log),
            observer,
            shutting_down: Arc::clone(&shutting_down),
        };
        std::thread::spawn(move || watchdog::run(&watch));
    }

    Ok(Handle::Owned(Owned {
        socket,
        log,
        current_pid,
        shutting_down,
        shutdown_called: AtomicBool::new(false),
    }))
}

fn spawn_config(
    program: &std::path::Path,
    socket: &std::path::Path,
    home: Option<&CrowbarHome>,
) -> SpawnConfig {
    let mut env = Vec::new();
    // Overridden home: export it so the daemon roots all its state there
    // too — this is what isolates a dev instance from the production
    // ~/.crowbar. Production keeps the env unset so the daemon resolves its
    // own default.
    if let Some(CrowbarHome {
        path,
        overridden: true,
    }) = home
    {
        let _ = std::fs::create_dir_all(path);
        env.push((
            "CROWBAR_HOME".to_owned(),
            path.to_string_lossy().into_owned(),
        ));
    }
    SpawnConfig {
        program: program.to_path_buf(),
        args: vec![
            "serve".to_owned(),
            "--host".to_owned(),
            format!("unix://{}", socket.display()),
        ],
        env,
    }
}

/// Retries `/v0/health` against a freshly spawned daemon, budgeted the same
/// way the original's `wait_for_health` was: bounded attempts, each with its
/// own timeout, so a daemon that accepts the connection but never serves
/// cannot park this call forever.
fn wait_for_health(
    socket: &std::path::Path,
    attempts: u32,
    per_attempt_timeout: Duration,
    retry_delay: Duration,
) -> Option<crowbar_client::Health> {
    for attempt in 0..attempts {
        if attempt > 0 {
            std::thread::sleep(retry_delay);
        }
        if let Ok(health) = crowbar_client::fetch_health_with_timeout(socket, per_attempt_timeout) {
            return Some(health);
        }
    }
    None
}

/// The reaper-and-respawner: blocks on the current generation's exit, then —
/// unless shutting down or out of restart budget — respawns and loops.
/// Unifies "the daemon died on its own" and "the watchdog SIGQUIT/SIGKILLed
/// it" into one path, since both end the same way: `spawned.wait()` returns.
fn supervise(
    mut spawned: process::Spawned,
    cfg: &SpawnConfig,
    log: &Arc<LogSink>,
    current_pid: &AtomicU32,
    shutting_down: &AtomicBool,
    observer: Option<&dyn watchdog::Observer>,
) {
    let mut budget = RestartBudget::new(RESTART_MAX, RESTART_WINDOW);
    loop {
        let exit = spawned.wait();
        log.write_line(
            "===",
            &format!(
                "daemon terminated (code={:?}, signal={:?})",
                exit.code, exit.signal
            ),
        );
        current_pid.store(0, Ordering::SeqCst);

        if shutting_down.load(Ordering::SeqCst) {
            return;
        }
        if !budget.allow(Instant::now()) {
            log.write_line(
                "===",
                &format!(
                    "daemon died {RESTART_MAX} times within {RESTART_WINDOW:?}; giving up until \
                     the next launch"
                ),
            );
            return;
        }

        std::thread::sleep(RESPAWN_DELAY);
        if shutting_down.load(Ordering::SeqCst) {
            return;
        }

        log.write_line("===", "respawning daemon after unexpected exit");
        match process::spawn(cfg, log) {
            Ok(new_spawned) => {
                current_pid.store(new_spawned.pid(), Ordering::SeqCst);
                if let Some(observer) = observer {
                    observer.on_event(watchdog::Event::Killed);
                }
                spawned = new_spawned;
            }
            Err(err) => {
                log.write_line("===", &format!("failed to respawn daemon: {err}"));
                return;
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use std::io::{BufRead as _, BufReader, Write as _};
    use std::os::unix::fs::PermissionsExt as _;
    use std::os::unix::net::{UnixListener, UnixStream};
    use std::path::{Path, PathBuf};
    use std::sync::OnceLock;
    use std::thread::JoinHandle;
    use std::time::Duration;

    use super::{EnsureError, Options, ensure_daemon};

    fn temp_dir(label: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "crowbar-sidecar-ensure-test-{label}-{}-{:?}",
            std::process::id(),
            std::thread::current().id(),
        ));
        std::fs::create_dir_all(&dir).expect("temp dir");
        dir
    }

    /// A **short**, flat socket path directly under the system temp dir —
    /// deliberately *not* nested inside [`temp_dir`]'s descriptively-named
    /// subdirectory. macOS caps `sockaddr_un.sun_path` at 104 bytes total,
    /// and this repository's own worktree path is long enough that
    /// `$TMPDIR/crowbar-sidecar-ensure-test-<label>-<pid>-<tid>/d.sock`
    /// already blows it — precisely the constraint `crate::socket` exists to
    /// route the daemon's *own* socket around (S0.3's own `sun_path` cap
    /// handling). A test fixture that ignores the same limit is not testing
    /// the crate honestly, so this helper applies it too.
    fn short_socket(tag: &str) -> PathBuf {
        std::env::temp_dir().join(format!("cbs-{tag}-{}.sock", std::process::id()))
    }

    /// A canned single-response `/v0/health` server, in-process — copied from
    /// `crowbar-client`'s own `OneShot` fixture. Enough to make one
    /// `fetch_health_with_timeout` call succeed, which is all the adopt-check
    /// itself ever does.
    struct OneShotHealth {
        socket: PathBuf,
        server: Option<JoinHandle<()>>,
    }

    impl OneShotHealth {
        fn listening(socket: PathBuf) -> Self {
            let _ = std::fs::remove_file(&socket);
            let listener = UnixListener::bind(&socket).expect("bind");
            let body = r#"{"success":true,"data":{"status":"ok","version":"test","pid":1}}"#;
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(),
                body,
            );
            let server = std::thread::spawn(move || {
                let Ok((stream, _)) = listener.accept() else {
                    return;
                };
                Self::answer(&stream, &response);
            });
            Self {
                socket,
                server: Some(server),
            }
        }

        fn answer(stream: &UnixStream, response: &str) {
            let mut reader = BufReader::new(stream);
            let mut line = String::new();
            while reader.read_line(&mut line).unwrap_or(0) > 0 {
                if line == "\r\n" || line == "\n" {
                    break;
                }
                line.clear();
            }
            let mut out = stream;
            let _ = out.write_all(response.as_bytes());
            let _ = out.flush();
        }
    }

    impl Drop for OneShotHealth {
        fn drop(&mut self) {
            if let Some(server) = self.server.take() {
                let _ = server.join();
            }
            let _ = std::fs::remove_file(&self.socket);
        }
    }

    /// A "daemon" that does nothing but prove it was *run*: an executable
    /// shell script that touches `marker` and exits. `SpawnConfig`'s
    /// `serve --host unix://…` args land as `$1 $2 $3` inside the script and
    /// are simply never referenced — a script does not care what argv it
    /// wasn't written to read.
    fn marker_script(dir: &Path, marker: &Path) -> PathBuf {
        let script = dir.join("marker-daemon.sh");
        std::fs::write(
            &script,
            format!("#!/bin/sh\ntouch \"{}\"\n", marker.display()),
        )
        .expect("write script");
        std::fs::set_permissions(&script, std::fs::Permissions::from_mode(0o755))
            .expect("chmod +x");
        script
    }

    #[test]
    fn adopting_a_healthy_daemon_never_spawns_anything() {
        let dir = temp_dir("adopt");
        let health = OneShotHealth::listening(short_socket("adopt"));
        let marker = dir.join("spawned.marker");
        let daemon_binary = marker_script(&dir, &marker);

        let handle = ensure_daemon(&Options {
            socket: Some(health.socket.clone()),
            daemon_binary: Some(daemon_binary),
            watchdog: false,
            ..Options::default()
        })
        .expect("the fake daemon answers healthy");

        assert!(!handle.is_owned(), "a healthy socket must be adopted");
        assert!(handle.pid().is_none(), "an adopted handle carries no pid");
        assert!(
            !marker.exists(),
            "adopt-if-healthy must not spawn anything when the socket already answers"
        );
    }

    /// **Mutation check for adopt-if-healthy**, performed and reverted by hand
    /// during S0.3: with the `if crowbar_client::fetch_health_with_timeout(...)
    /// .is_ok() { return Ok(Handle::Adopted { socket }); }` branch at the top
    /// of `ensure_daemon` commented out (so the function always fell through
    /// to spawning), this exact test went red — `marker.exists()` became true
    /// and `handle.is_owned()` became true against the very same healthy
    /// socket. Restoring the branch turned it green again. See this crate's
    /// PR/commit history for the diff; this comment is the record that the
    /// exercise was actually run, not merely described.
    /// Checking `marker.exists()` after `ensure_daemon` returns would race the
    /// production `NeverBecameHealthy` path, which SIGKILLs the process it
    /// gave up on: under a busy parallel test run, that signal can land
    /// before the marker script (a real `fork`+`exec`+`touch`, not free) gets
    /// scheduled. So this checks the daemon log's `"daemon spawned"` line
    /// instead — `process::spawn` writes it synchronously, the instant the OS
    /// confirms the child started, strictly before `ensure_daemon` does
    /// anything else (including the eventual kill). That line existing is
    /// unconditional proof a spawn was attempted; nothing about how long the
    /// child then lived can make it disappear.
    #[test]
    fn spawning_when_nothing_answers_actually_runs_the_binary() {
        let dir = temp_dir("spawn");
        let dead_socket = short_socket("spawn-dead");
        let marker = dir.join("spawned.marker");
        let daemon_binary = marker_script(&dir, &marker);

        let result = ensure_daemon(&Options {
            socket: Some(dead_socket),
            daemon_binary: Some(daemon_binary),
            // Pinned to this test's own directory rather than the real
            // dev-fallback home: keeps the daemon log (and this assertion)
            // out of the checkout's actual `.crowbar/`.
            home: Some(crate::home::CrowbarHome {
                path: dir.clone(),
                overridden: true,
            }),
            watchdog: false,
            // The stand-in never serves `/v0/health`, so giving up is bounded
            // well under the full ~6s production budget for a negative
            // result this test already expects.
            health_probe_attempts: 5,
            health_probe_timeout: Duration::from_millis(50),
            health_probe_retry_delay: Duration::from_millis(10),
            ..Options::default()
        });

        let log = std::fs::read_to_string(dir.join("logs").join("daemon.log")).unwrap_or_default();
        assert!(
            log.contains("daemon spawned"),
            "a dead socket must fall through to actually spawning the daemon binary; log: {log}"
        );
        assert!(
            matches!(result, Err(EnsureError::NeverBecameHealthy)),
            "the stand-in never serves /v0/health, so this must be reported, not silently ok: {result:?}",
        );
    }

    #[test]
    fn a_missing_daemon_binary_is_a_reported_error_not_a_panic() {
        let _dir = temp_dir("missing-binary");
        let dead_socket = short_socket("missing-binary");

        let result = ensure_daemon(&Options {
            socket: Some(dead_socket),
            daemon_binary: Some(PathBuf::from("/this/does/not/exist/crowbar-api")),
            watchdog: false,
            health_probe_attempts: 1,
            health_probe_timeout: Duration::from_millis(50),
            health_probe_retry_delay: Duration::from_millis(1),
            ..Options::default()
        });

        assert!(matches!(result, Err(EnsureError::Spawn(_))), "{result:?}");
    }

    /// Compiles the one genuine stand-in daemon this test module needs: a
    /// real OS process, with its own pid, that binds the socket it is told
    /// and actually answers `/v0/health` — needed for
    /// [`an_adopted_daemon_survives_the_adopters_shutdown`], which has to
    /// prove a *second* `ensure_daemon` call really did adopt a live process
    /// rather than merely observing one in-process fixture thread.
    ///
    /// Built with `rustc` rather than shipped as a `src/bin/` target: a
    /// `[[bin]]` here would make `cargo build --workspace` — one of the four
    /// gates every item is measured against — produce an extra artifact that
    /// exists for exactly one test and nothing this crate ships. `rustc` is
    /// guaranteed on `PATH` in this environment: it is the same toolchain
    /// already required to run this test at all (`native/README.md`'s own
    /// `PATH` note).
    fn compiled_fake_daemon() -> &'static Path {
        static BIN: OnceLock<PathBuf> = OnceLock::new();
        BIN.get_or_init(|| {
            let dir = temp_dir("fake-daemon-build");
            let src = dir.join("fake_daemon.rs");
            std::fs::write(&src, FAKE_DAEMON_SOURCE).expect("write fake daemon source");
            let bin = dir.join("fake-daemon");
            let status = std::process::Command::new("rustc")
                .args(["-O", "-o"])
                .arg(&bin)
                .arg(&src)
                .status()
                .expect("rustc is on PATH — see native/README.md's PATH note");
            assert!(
                status.success(),
                "failed to compile the fake daemon fixture"
            );
            bin
        })
    }

    /// Binds the unix socket named by `--host unix://<path>` and serves
    /// `/v0/health` indefinitely (until killed), so a real `ensure_daemon`
    /// spawn-and-adopt round trip has something genuine to talk to.
    const FAKE_DAEMON_SOURCE: &str = r#"
use std::io::{Read, Write};
use std::os::unix::net::UnixListener;

fn main() {
    let mut socket_path = None;
    let mut args = std::env::args().skip(1);
    while let Some(arg) = args.next() {
        if arg == "--host" {
            if let Some(host) = args.next() {
                socket_path = host.strip_prefix("unix://").map(|s| s.to_string());
            }
        }
    }
    let path = socket_path.expect("no --host unix://<path> given");
    let _ = std::fs::remove_file(&path);
    let listener = UnixListener::bind(&path).expect("bind");
    let body = format!(
        "{{\"success\":true,\"data\":{{\"status\":\"ok\",\"version\":\"fake\",\"pid\":{}}}}}",
        std::process::id()
    );
    let response = format!(
        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
        body.len(),
        body
    );
    for stream in listener.incoming() {
        let Ok(mut stream) = stream else { continue };
        let mut buf = [0u8; 1024];
        let _ = stream.read(&mut buf);
        let _ = stream.write_all(response.as_bytes());
    }
}
"#;

    /// The property adopt-if-healthy exists to guarantee: an app that
    /// *adopted* a daemon it did not start must not be able to end it.
    #[test]
    fn an_adopted_daemon_survives_the_adopters_shutdown() {
        let bin = compiled_fake_daemon();
        let dir = temp_dir("survive");
        let socket = short_socket("survive");

        // Health-probe budget left at `Options::default()`'s production
        // values (30 attempts × 200ms) rather than tightened: this test's
        // whole point is a *real* compiled process actually binding and
        // serving, and under a fully parallel `cargo test` run — this test
        // plus everything else in the suite competing for the CPU, on top of
        // the `rustc` invocation `compiled_fake_daemon` itself just paid —
        // that can genuinely take longer than a tight budget allows. The
        // happy path still returns the moment the daemon answers; only a
        // wedged/never-arriving daemon would pay the full budget, and that
        // is exactly the case that should be reported, not raced against.
        let owner = ensure_daemon(&Options {
            socket: Some(socket.clone()),
            daemon_binary: Some(bin.to_path_buf()),
            // Pinned to this test's own directory rather than the real
            // dev-fallback home, so its daemon log does not land in the
            // checkout's actual `.crowbar/`.
            home: Some(crate::home::CrowbarHome {
                path: dir.clone(),
                overridden: true,
            }),
            watchdog: false,
            ..Options::default()
        })
        .expect("the compiled fake daemon becomes healthy");
        assert!(owner.is_owned());
        let owner_pid = owner.pid().expect("an owned handle always has a pid");
        assert!(crate::process::exists(owner_pid));

        // A second call against the same, now-live socket: this is the
        // "both apps against one CROWBAR_HOME" scenario itself.
        let adopter = ensure_daemon(&Options {
            socket: Some(socket.clone()),
            daemon_binary: Some(bin.to_path_buf()),
            watchdog: false,
            ..Options::default()
        })
        .expect("the owner's daemon is already healthy");
        assert!(
            !adopter.is_owned(),
            "the second caller must adopt, not spawn a second daemon"
        );

        adopter.shutdown("adopter exiting");
        drop(adopter);

        assert!(
            crate::process::exists(owner_pid),
            "shutting down an ADOPTED handle must never kill the daemon the OWNER started"
        );
        assert!(
            crowbar_client::fetch_health(&socket).is_ok(),
            "the owner's daemon must still be serving after the adopter's shutdown"
        );

        // Clean up the real process this test actually started.
        owner.shutdown("test cleanup");
        drop(owner);
        assert!(!crate::process::exists(owner_pid));
        let _ = std::fs::remove_file(&socket);
        let _ = std::fs::remove_dir_all(&dir);
    }
}
