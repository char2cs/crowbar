use std::path::PathBuf;
use std::sync::Mutex;
use std::time::Duration;

use http_body_util::{BodyExt, Empty};
use hyper::Request;
use hyper_util::rt::TokioIo;
use tauri::{AppHandle, Manager, Runtime};
use tauri_plugin_shell::{process::CommandChild, ShellExt};
use tokio::net::UnixStream;

/// Holds the child process handle for the crowbar-api sidecar so it can be
/// killed cleanly when the Tauri window closes, plus the path to the unix
/// socket the daemon is listening on so lib.rs and the api_proxy can reach it.
pub struct SidecarHandle {
    pub child: Mutex<Option<CommandChild>>,
    pub socket_path: Mutex<Option<PathBuf>>,
}

impl SidecarHandle {
    pub fn new() -> Self {
        Self {
            child: Mutex::new(None),
            socket_path: Mutex::new(None),
        }
    }

    /// Returns the daemon's unix-socket path, if the sidecar has been spawned.
    pub fn socket_path(&self) -> Option<PathBuf> {
        self.socket_path.lock().unwrap().clone()
    }
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

    let (_rx, child) = sidecar.spawn()?;

    // Store child + socket path in managed state so it can be killed on window
    // close and so the api_proxy / lib.rs can locate the socket.
    {
        let state = app.state::<SidecarHandle>();
        state.child.lock().unwrap().replace(child);
        state.socket_path.lock().unwrap().replace(socket.clone());
    }

    wait_for_health(&socket, 30).await?;
    log::info!("crowbar daemon is ready on {}", socket.display());
    Ok(())
}

async fn wait_for_health(
    socket: &PathBuf,
    attempts: u32,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    for i in 0..attempts {
        tokio::time::sleep(Duration::from_millis(200)).await;

        if check_health(socket).await.is_ok() {
            return Ok(());
        }

        if i == attempts - 1 {
            return Err("daemon did not become healthy within 6s".into());
        }
    }
    Ok(())
}

async fn check_health(socket: &PathBuf) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
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
        .uri("/v0/health")
        .header("Host", "localhost")
        .body(Empty::<bytes::Bytes>::new())?;

    let resp = sender.send_request(req).await?;
    if resp.status().is_success() {
        // Drain the body so the connection can be reused/closed cleanly.
        let _ = resp.into_body().collect().await;
        Ok(())
    } else {
        Err(format!("health check returned {}", resp.status()).into())
    }
}

#[cfg(test)]
mod tests {
    use super::{fnv1a64, override_socket_path, socket_path};

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
