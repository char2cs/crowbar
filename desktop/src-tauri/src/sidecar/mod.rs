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

/// The daemon's unix-socket path: a fixed, well-known location under the user's
/// home directory (`~/.crowbar/crowbar.sock`), matching the default the daemon
/// itself resolves for `unix://`. A fixed path — rather than a per-process temp
/// path — means the proxy always knows where to reach the daemon, and the
/// daemon's own stale-socket handling (dial-to-detect + reclaim, unlink on
/// clean shutdown) keeps it healthy across restarts instead of leaving a trail
/// of dead per-PID sockets in the temp dir.
pub fn socket_path() -> PathBuf {
    crowbar_home()
        .unwrap_or_else(std::env::temp_dir)
        .join(DEFAULT_SOCKET_NAME)
}

const DEFAULT_SOCKET_NAME: &str = "crowbar.sock";

/// `~/.crowbar`, resolving the home directory the same way the Go daemon does
/// (`os.UserHomeDir`): `$HOME` on unix, `%USERPROFILE%` on Windows.
fn crowbar_home() -> Option<PathBuf> {
    #[cfg(windows)]
    let home = std::env::var_os("USERPROFILE");
    #[cfg(not(windows))]
    let home = std::env::var_os("HOME");
    home.map(|h| PathBuf::from(h).join(".crowbar"))
}

pub async fn spawn<R: Runtime>(
    app: &AppHandle<R>,
    socket: PathBuf,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    // Ensure the socket's parent dir (~/.crowbar) exists; the daemon binds the
    // socket inside it. We deliberately do NOT pre-remove the socket file: the
    // daemon's own stale-socket handling reclaims a dead socket and refuses to
    // clobber one with a live daemon still behind it.
    if let Some(dir) = socket.parent() {
        let _ = std::fs::create_dir_all(dir);
    }

    let host = format!("unix://{}", socket.display());
    let sidecar = app
        .shell()
        .sidecar("crowbar-api")?
        .args(["serve", "--host", &host]);

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

async fn check_health(
    socket: &PathBuf,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
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
    use super::socket_path;

    #[test]
    fn socket_path_is_fixed_crowbar_sock_under_home() {
        let p = socket_path();
        let name = p.file_name().unwrap().to_string_lossy().into_owned();
        assert_eq!(name, "crowbar.sock", "got {name}");

        // The socket lives under the fixed ~/.crowbar dir, not a per-PID temp
        // path, so the proxy can always find it.
        let parent = p.parent().unwrap().file_name().unwrap().to_string_lossy();
        assert_eq!(parent, ".crowbar", "got {parent}");
    }
}
