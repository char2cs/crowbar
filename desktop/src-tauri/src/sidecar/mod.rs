pub mod health;
pub mod paths;
pub mod stop;
pub mod supervisor;
pub mod watchdog;

pub use paths::{daemon_log_path, resolve, socket_path};
pub use stop::shutdown;
pub use watchdog::start_watchdog;

use health::{announce_halt, wait_for_health};

use std::io::Write;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicI32, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use http_body_util::Empty;
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
/// At most this many automatic respawns per window; beyond it the daemon is
/// declared dead until the next app launch (guards against crash loops).
const RESTART_MAX: usize = 3;
const RESTART_WINDOW: Duration = Duration::from_secs(600);
/// Holds the child process handle for the crowbar-api sidecar so it can be
/// killed cleanly when the Tauri window closes, plus the path to the unix
/// socket the daemon is listening on so lib.rs and the api_proxy can reach it.
pub struct SidecarHandle {
    pub child: Mutex<Option<CommandChild>>,
    pub socket_path: Mutex<Option<PathBuf>>,
    /// Set by the window-close kill path so the supervisor never respawns a
    /// daemon the user is intentionally shutting down.
    pub shutting_down: AtomicBool,
    /// The pid of the daemon process WE spawned; 0 = none (not spawned, or exited).
    ///
    /// Recorded from the child handle the instant `spawn` returns — never from
    /// /v0/health, which answers for whatever process holds the socket (a stale
    /// daemon from a previous run, say), and so could name a process we must not
    /// signal. Every kill path signals THIS pid via libc rather than calling into
    /// the `CommandChild`, whose kill()/pid() take the shared_child lock.
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

    /// The pid of the daemon we spawned, while it is running.
    pub fn daemon_pid(&self) -> Option<i32> {
        match self.daemon_pid.load(Ordering::SeqCst) {
            0 => None,
            pid => Some(pid),
        }
    }
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
    // The same dev home `resolve` gave `paths`, so the daemon roots its state
    // where the shell expects its socket.
    let sidecar = paths::with_dev_home(
        app.shell()
            .sidecar("crowbar-api")?
            .args(["serve", "--host", &host]),
    );

    let (rx, child) = sidecar.spawn()?;
    // Read once, here: the plugin's wait thread only holds the shared_child lock
    // momentarily (shared_child >= 1.1 releases it before blocking in waitid), so
    // this does not contend with it. From here on the pid is known, so a stop that
    // lands while the daemon is still booting is a SIGTERM too, not a bare kill.
    let spawned_pid = i32::try_from(child.pid()).unwrap_or(0);

    // Store child + socket path in managed state so it can be killed on window
    // close and so the api_proxy / lib.rs can locate the socket.
    {
        let state = app.state::<SidecarHandle>();
        state.child.lock().unwrap().replace(child);
        state.socket_path.lock().unwrap().replace(socket.clone());
        state.daemon_pid.store(spawned_pid, Ordering::SeqCst);
    }

    // Capture the daemon's stdout/stderr into the daemon log and supervise its
    // exit — without this pump a Go panic's stack trace vanishes with the pipe.
    let pump_app = app.clone();
    tauri::async_runtime::spawn(pump_output(pump_app, rx));

    let pid = wait_for_health(&socket, 30)
        .await
        .inspect_err(|why| announce_halt(app, why))?;
    if let Some(reported) = pid.filter(|reported| *reported != spawned_pid) {
        // Someone else answers on our socket — a stale daemon from a previous run.
        // It is not ours to signal; our own process is what a stop must end.
        log::warn!(
            "daemon on {} reports pid {reported}, but we spawned {spawned_pid}",
            socket.display()
        );
    }
    log::info!(
        "crowbar daemon is ready on {} (pid {spawned_pid})",
        socket.display()
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
    // The pid dies with the child: a recycled pid must never be signalled.
    state.daemon_pid.store(0, Ordering::SeqCst);

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
