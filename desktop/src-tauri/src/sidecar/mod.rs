pub mod supervisor;

use std::io::Write;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicI32, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use http_body_util::{BodyExt, Empty};
use hyper::Request;
use hyper_util::rt::TokioIo;
use tauri::{AppHandle, Emitter, Manager, Runtime};
use tauri_plugin_shell::{
    process::{CommandChild, CommandEvent},
    ShellExt,
};
use tokio::net::UnixStream;

/// Cap per daemon-log generation; one previous generation is kept as `.1`.
const DAEMON_LOG_MAX_LEN: u64 = 4 * 1024 * 1024;
/// How often the watchdog probes the daemon's deep readiness path.
const WATCHDOG_INTERVAL: Duration = Duration::from_secs(10);
/// Per-probe budget; the deep path answers in microseconds when healthy.
const PROBE_TIMEOUT: Duration = Duration::from_secs(5);
/// A final, more generous probe budget after the failure threshold trips,
/// giving a daemon that just resumed from an OS suspension time to answer
/// before the watchdog commits to a restart.
const SUSPEND_GRACE_PROBE: Duration = Duration::from_secs(8);
/// Consecutive failed probes before the watchdog declares a wedge.
const PROBE_FAILURE_THRESHOLD: u32 = 3;
/// At most this many automatic respawns per window; beyond it the daemon is
/// declared dead until the next app launch (guards against crash loops).
const RESTART_MAX: usize = 3;
const RESTART_WINDOW: Duration = Duration::from_secs(600);
/// Grace between SIGQUIT (Go dumps goroutines to captured stderr) and SIGKILL.
const SIGQUIT_GRACE: Duration = Duration::from_secs(2);

/// Holds the child process handle for the crowbar-api sidecar so it can be
/// killed cleanly when the Tauri window closes, plus the path to the unix
/// socket the daemon is listening on so lib.rs and the api_proxy can reach it.
pub struct SidecarHandle {
    pub child: Mutex<Option<CommandChild>>,
    pub socket_path: Mutex<Option<PathBuf>>,
    /// Set by the window-close kill path so the supervisor never respawns a
    /// daemon the user is intentionally shutting down.
    pub shutting_down: AtomicBool,
    /// The daemon's pid as self-reported by /v0/health at spawn; 0 = unknown.
    ///
    /// Every kill path signals THIS pid via libc, never `CommandChild::pid()`:
    /// that call locks the shared_child mutex the shell plugin's wait thread
    /// holds for the child's entire lifetime, and deadlocks on a live daemon
    /// (observed live 2026-07-04 — the watchdog froze mid-kill).
    daemon_pid: AtomicI32,
    restart_budget: Mutex<supervisor::RestartBudget>,
}

impl SidecarHandle {
    pub fn new() -> Self {
        Self {
            child: Mutex::new(None),
            socket_path: Mutex::new(None),
            shutting_down: AtomicBool::new(false),
            daemon_pid: AtomicI32::new(0),
            restart_budget: Mutex::new(supervisor::RestartBudget::new(RESTART_MAX, RESTART_WINDOW)),
        }
    }

    /// Returns the daemon's unix-socket path, if the sidecar has been spawned.
    pub fn socket_path(&self) -> Option<PathBuf> {
        self.socket_path.lock().unwrap().clone()
    }

    /// The daemon's self-reported pid, if health reporting has captured one.
    pub fn daemon_pid(&self) -> Option<i32> {
        match self.daemon_pid.load(Ordering::SeqCst) {
            0 => None,
            pid => Some(pid),
        }
    }
}

/// The daemon's captured stdout/stderr: `<crowbar home>/logs/daemon.log`.
/// This is the only place a Go panic's stack trace survives in the packaged
/// app — the sidecar's pipes are otherwise connected to nothing.
pub fn daemon_log_path() -> PathBuf {
    let (home, _) = crowbar_home();
    home.unwrap_or_else(std::env::temp_dir)
        .join("logs")
        .join("daemon.log")
}

fn open_daemon_log() -> Option<std::fs::File> {
    let path = daemon_log_path();
    if let Some(dir) = path.parent() {
        let _ = std::fs::create_dir_all(dir);
    }
    let _ = supervisor::rotate_if_needed(&path, DAEMON_LOG_MAX_LEN);
    std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(&path)
        .ok()
}

fn epoch_secs() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

/// The daemon's unix-socket path: a fixed, well-known location matching the
/// default the daemon itself resolves for `unix://`. A fixed path — rather
/// than a per-process temp path — means the proxy always knows where to reach
/// the daemon, and the daemon's own stale-socket handling (dial-to-detect +
/// reclaim, unlink on clean shutdown) keeps it healthy across restarts instead
/// of leaving a trail of dead per-PID sockets in the temp dir.
///
/// Production: `~/.crowbar/crowbar.sock`. Overridden homes (CROWBAR_HOME env
/// or the dev-build workspace default): a short home-keyed name in the temp
/// dir (see [`override_socket_path`]) — the socket cannot live inside the
/// override home because macOS caps sun_path at 104 bytes and workspace
/// worktree paths exceed it.
pub fn socket_path() -> PathBuf {
    let (home, overridden) = crowbar_home();
    match (home, overridden) {
        (Some(h), true) => override_socket_path(&h),
        (Some(h), false) => h.join(DEFAULT_SOCKET_NAME),
        (None, _) => std::env::temp_dir().join(DEFAULT_SOCKET_NAME),
    }
}

const DEFAULT_SOCKET_NAME: &str = "crowbar.sock";

/// The crowbar home root plus whether it is an override of the production
/// default, mirroring the Go daemon's resolution order:
/// 1. `CROWBAR_HOME` env override (must be an absolute path) — override.
/// 2. Dev builds only: `.crowbar` inside the workspace being developed
///    (`<repo root>/.crowbar`, derived from CARGO_MANIFEST_DIR at compile
///    time), so a dev instance never shares state or the control socket with
///    the production app in `~/.crowbar` — override.
/// 3. `~/.crowbar` (`$HOME` on unix, `%USERPROFILE%` on Windows) — production.
fn crowbar_home() -> (Option<PathBuf>, bool) {
    if let Some(dir) = std::env::var_os("CROWBAR_HOME") {
        if !dir.is_empty() {
            return (Some(PathBuf::from(dir)), true);
        }
    }

    #[cfg(debug_assertions)]
    if let Some(root) = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|p| p.parent())
    {
        return (Some(root.join(".crowbar")), true);
    }

    #[cfg(windows)]
    let home = std::env::var_os("USERPROFILE");
    #[cfg(not(windows))]
    let home = std::env::var_os("HOME");
    (home.map(|h| PathBuf::from(h).join(".crowbar")), false)
}

/// Socket path for an overridden crowbar home: `crowbar-<fnv1a64(home)>.sock`
/// in the temp dir. MUST stay byte-identical to the Go daemon's derivation
/// (overrideSocketPath in api/internal/core/gateway/transports/socket.go) so a
/// daemon restarted manually with CROWBAR_HOME set binds exactly where this
/// proxy dials. The hash input is the home path string exactly as exported in
/// the CROWBAR_HOME env var.
fn override_socket_path(home: &std::path::Path) -> PathBuf {
    let hash = fnv1a64(home.to_string_lossy().as_bytes());
    std::env::temp_dir().join(format!("crowbar-{hash:x}.sock"))
}

/// FNV-1a 64-bit, matching Go's hash/fnv New64a.
fn fnv1a64(bytes: &[u8]) -> u64 {
    let mut h: u64 = 0xcbf2_9ce4_8422_2325;
    for b in bytes {
        h ^= u64::from(*b);
        h = h.wrapping_mul(0x0000_0100_0000_01b3);
    }
    h
}

pub async fn spawn<R: Runtime>(
    app: &AppHandle<R>,
    socket: PathBuf,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    // Ensure the socket's parent dir exists; the daemon binds the socket
    // inside it. We deliberately do NOT pre-remove the socket file: the
    // daemon's own stale-socket handling reclaims a dead socket and refuses to
    // clobber one with a live daemon still behind it.
    if let Some(dir) = socket.parent() {
        let _ = std::fs::create_dir_all(dir);
    }

    let host = format!("unix://{}", socket.display());
    let mut sidecar = app
        .shell()
        .sidecar("crowbar-api")?
        .args(["serve", "--host", &host]);

    // Overridden home (CROWBAR_HOME env or dev-build workspace default):
    // export it so the daemon roots all its state (projects, store, logs)
    // there too — this is what isolates a dev instance from the production
    // ~/.crowbar. Production keeps the env unset so the daemon resolves its
    // own default.
    let (home, overridden) = crowbar_home();
    if overridden {
        if let Some(home) = home {
            let _ = std::fs::create_dir_all(&home);
            sidecar = sidecar.env("CROWBAR_HOME", home.to_string_lossy().to_string());
        }
    }

    let (rx, child) = sidecar.spawn()?;

    // Store child + socket path in managed state so it can be killed on window
    // close and so the api_proxy / lib.rs can locate the socket.
    {
        let state = app.state::<SidecarHandle>();
        state.child.lock().unwrap().replace(child);
        state.socket_path.lock().unwrap().replace(socket.clone());
    }

    // Capture the daemon's stdout/stderr into the daemon log and supervise its
    // exit — without this pump a Go panic's stack trace vanishes with the pipe.
    let pump_app = app.clone();
    tauri::async_runtime::spawn(pump_output(pump_app, rx));

    let pid = wait_for_health(&socket, 30).await?;
    {
        let state = app.state::<SidecarHandle>();
        state.daemon_pid.store(pid.unwrap_or(0), Ordering::SeqCst);
    }
    log::info!(
        "crowbar daemon is ready on {} (pid {:?})",
        socket.display(),
        pid
    );
    Ok(())
}

/// Drains the sidecar's output channel into the daemon log. On termination it
/// records the exit code/signal — the datum every past silent daemon death was
/// missing — and hands off to the respawn policy.
async fn pump_output<R: Runtime>(
    app: AppHandle<R>,
    mut rx: tauri::async_runtime::Receiver<CommandEvent>,
) {
    let mut log_file = open_daemon_log();
    let mut lines_since_rotate_check: u32 = 0;
    let write_line = |file: &mut Option<std::fs::File>, prefix: &str, text: &str| {
        if let Some(f) = file {
            let _ = writeln!(f, "{prefix} {}", text.trim_end_matches(['\r', '\n']));
        }
    };
    write_line(
        &mut log_file,
        "===",
        &format!("daemon spawned (epoch {})", epoch_secs()),
    );

    while let Some(event) = rx.recv().await {
        // The Go daemon logs every request to stderr, so one long-lived daemon
        // can outgrow the cap between spawns; re-check size periodically.
        lines_since_rotate_check += 1;
        if lines_since_rotate_check >= 5000 {
            lines_since_rotate_check = 0;
            if supervisor::rotate_if_needed(&daemon_log_path(), DAEMON_LOG_MAX_LEN).is_ok() {
                log_file = open_daemon_log();
            }
        }
        match event {
            CommandEvent::Stdout(bytes) => {
                write_line(&mut log_file, "[out]", &String::from_utf8_lossy(&bytes));
            }
            CommandEvent::Stderr(bytes) => {
                write_line(&mut log_file, "[err]", &String::from_utf8_lossy(&bytes));
            }
            CommandEvent::Error(e) => {
                write_line(&mut log_file, "[evt]", &format!("channel error: {e}"));
            }
            CommandEvent::Terminated(payload) => {
                let summary = format!(
                    "daemon terminated (epoch {}, code={:?}, signal={:?})",
                    epoch_secs(),
                    payload.code,
                    payload.signal
                );
                write_line(&mut log_file, "===", &summary);
                log::error!("crowbar {summary}");
                let _ = app.emit("daemon:terminated", summary.clone());
                handle_termination(&app);
                break;
            }
            _ => {}
        }
    }
}

/// Decides what happens after the daemon exits: nothing during an intentional
/// shutdown, a budgeted respawn otherwise. Runs the respawn as a boxed task —
/// spawn() owns the pump future, so re-entering it from the pump must cross a
/// type-erased boundary.
fn handle_termination<R: Runtime>(app: &AppHandle<R>) {
    let state = app.state::<SidecarHandle>();
    state.child.lock().unwrap().take();

    if state.shutting_down.load(Ordering::SeqCst) {
        return;
    }
    if !state.restart_budget.lock().unwrap().allow(Instant::now()) {
        log::error!(
            "crowbar daemon died {RESTART_MAX} times within {RESTART_WINDOW:?}; \
             giving up until the next app launch"
        );
        let _ = app.emit("daemon:dead", ());
        return;
    }

    let app = app.clone();
    let respawn: std::pin::Pin<Box<dyn std::future::Future<Output = ()> + Send>> =
        Box::pin(async move {
            tokio::time::sleep(Duration::from_millis(500)).await;
            let state = app.state::<SidecarHandle>();
            if state.shutting_down.load(Ordering::SeqCst) {
                return;
            }
            log::warn!("respawning crowbar daemon after unexpected exit");
            match spawn(&app, socket_path()).await {
                Ok(()) => {
                    let _ = app.emit("daemon:restarted", ());
                }
                Err(e) => log::error!("failed to respawn crowbar daemon: {e}"),
            }
        });
    tauri::async_runtime::spawn(respawn);
}

async fn wait_for_health(
    socket: &PathBuf,
    attempts: u32,
) -> Result<Option<i32>, Box<dyn std::error::Error + Send + Sync>> {
    for i in 0..attempts {
        tokio::time::sleep(Duration::from_millis(200)).await;

        if let Ok(pid) = check_health(socket).await {
            return Ok(pid);
        }

        if i == attempts - 1 {
            return Err("daemon did not become healthy within 6s".into());
        }
    }
    Ok(None)
}

/// Probes /v0/health; on success returns the daemon's self-reported pid
/// (None for a daemon predating the pid field).
async fn check_health(
    socket: &PathBuf,
) -> Result<Option<i32>, Box<dyn std::error::Error + Send + Sync>> {
    let resp = http_get(socket, "/v0/health").await?;
    if resp.status().is_success() {
        let body = resp.into_body().collect().await?.to_bytes();
        Ok(parse_health_pid(&body))
    } else {
        Err(format!("health check returned {}", resp.status()).into())
    }
}

/// One HTTP/1.1 GET over the daemon's unix socket.
pub(crate) async fn http_get(
    socket: &PathBuf,
    path: &str,
) -> Result<hyper::Response<hyper::body::Incoming>, Box<dyn std::error::Error + Send + Sync>> {
    let stream = UnixStream::connect(socket).await?;
    let io = TokioIo::new(stream);

    let (mut sender, conn) = hyper::client::conn::http1::handshake(io).await?;
    // Drive the connection in the background; it completes when the request is done.
    tokio::spawn(async move {
        let _ = conn.await;
    });

    // Authority is irrelevant over a unix socket but hyper requires a valid
    // Host header for HTTP/1.1, so use a placeholder.
    let req = Request::builder()
        .method("GET")
        .uri(path)
        .header("Host", "localhost")
        .body(Empty::<bytes::Bytes>::new())?;

    Ok(sender.send_request(req).await?)
}

/// Deep readiness probe. Unlike /v0/health — which answers from a static
/// handler and stays green while the daemon is wedged — /v0/projects goes
/// through the global view store, the exact resource every observed
/// production wedge pinned. A daemon that cannot answer this is not serving
/// users, whatever its liveness endpoint says.
///
/// Dialling costs a descriptor of *this* process, so the probe reports whether it
/// could dial at all: an exhausted app cannot reach even a perfectly healthy
/// daemon, and the watchdog must not read that as the daemon's fault.
async fn probe_ready(socket: &PathBuf) -> supervisor::Probe {
    match http_get(socket, "/v0/projects").await {
        Ok(resp) => {
            let ok = resp.status().is_success();
            let _ = resp.into_body().collect().await;
            if ok {
                supervisor::Probe::Healthy
            } else {
                supervisor::Probe::Unserved
            }
        }
        Err(e) if is_descriptor_exhaustion(&*e) => supervisor::Probe::LocalDescriptorExhaustion,
        Err(_) => supervisor::Probe::Unserved,
    }
}

/// Whether a dial failed because this process is out of file descriptors. hyper
/// wraps the `connect(2)` error, so the cause chain has to be walked.
fn is_descriptor_exhaustion(err: &(dyn std::error::Error + 'static)) -> bool {
    let mut cause = Some(err);
    while let Some(e) = cause {
        if let Some(io) = e.downcast_ref::<std::io::Error>() {
            if matches!(io.raw_os_error(), Some(libc::EMFILE) | Some(libc::ENFILE)) {
                return true;
            }
        }
        cause = e.source();
    }
    false
}

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
            if !tracker.observe(outcome == supervisor::Probe::Healthy) {
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
    let (child, pid) = {
        let state = app.state::<SidecarHandle>();
        let child = state.child.lock().unwrap().take();
        (child, state.daemon_pid())
    };
    let Some(child) = child else { return };
    #[cfg(unix)]
    {
        match pid {
            Some(pid) => {
                let pid = pid as libc::pid_t;
                unsafe { libc::kill(pid, libc::SIGQUIT) };
                tokio::time::sleep(SIGQUIT_GRACE).await;
                // No-op (ESRCH) if the SIGQUIT dump already ended it.
                unsafe { libc::kill(pid, libc::SIGKILL) };
                drop(child);
            }
            None => {
                // Daemon predating pid reporting: CommandChild::kill is the
                // only lever left, deadlock risk and all.
                log::warn!("daemon pid unknown; falling back to CommandChild::kill");
                let _ = child.kill();
            }
        }
    }
    #[cfg(not(unix))]
    {
        let _ = pid;
        let _ = child.kill();
    }
}

/// Extracts the daemon's self-reported pid from the /v0/health envelope:
/// `{"success":true,"data":{"status":"ok","version":"...","pid":<n>}}`.
///
/// This is the ONLY safe pid source: `CommandChild::pid()` locks the
/// shared_child mutex that the shell plugin's wait thread holds for the
/// child's entire lifetime — calling it on a live daemon deadlocks (observed
/// live 2026-07-04: the watchdog froze inside pid() during the wedge drill).
fn parse_health_pid(body: &[u8]) -> Option<i32> {
    let v: serde_json::Value = serde_json::from_slice(body).ok()?;
    let pid = v.get("data")?.get("pid")?.as_i64()?;
    i32::try_from(pid).ok().filter(|p| *p > 0)
}

#[cfg(test)]
mod tests {
    use super::{fnv1a64, override_socket_path, parse_health_pid, socket_path};

    #[test]
    fn parse_health_pid_reads_the_envelope() {
        let body = br#"{"success":true,"data":{"status":"ok","version":"0.1.0","pid":54321}}"#;
        assert_eq!(parse_health_pid(body), Some(54321));
    }

    #[test]
    fn parse_health_pid_rejects_missing_or_invalid() {
        assert_eq!(
            parse_health_pid(br#"{"success":true,"data":{"status":"ok"}}"#),
            None,
            "old daemons without a pid field must not yield one"
        );
        assert_eq!(parse_health_pid(b"not json"), None);
        assert_eq!(
            parse_health_pid(br#"{"success":true,"data":{"pid":0}}"#),
            None,
            "pid 0 would signal the whole process group — never accept it"
        );
    }

    #[test]
    fn socket_path_is_fixed_and_deterministic() {
        // cfg(test) implies debug_assertions, so this exercises the dev
        // override branch: a short home-keyed socket in the temp dir (the
        // workspace home itself exceeds macOS's 104-byte sun_path cap).
        let p = socket_path();
        let name = p.file_name().unwrap().to_string_lossy().into_owned();
        assert!(
            name.starts_with("crowbar-") && name.ends_with(".sock"),
            "got {name}"
        );
        // Deterministic: the proxy must always find the daemon at the same path.
        assert_eq!(p, socket_path());
    }

    // Pins the fnv1a64 hash so this derivation and the Go daemon's
    // (overrideSocketPath in api/.../transports/socket.go, pinned by
    // TestOverrideSocketPath_MatchesDesktopDerivation) can never drift.
    #[test]
    fn override_socket_path_matches_go_derivation() {
        assert_eq!(fnv1a64(b"/dev/crowbar-home"), 0xc13f09536446a88e);
        let p = override_socket_path(std::path::Path::new("/dev/crowbar-home"));
        assert_eq!(
            p.file_name().unwrap().to_string_lossy(),
            "crowbar-c13f09536446a88e.sock"
        );
    }
}
