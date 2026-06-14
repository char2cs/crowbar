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

/// Compute a per-process unix-socket path under the OS temp dir. Using the PID
/// keeps it unique per launch so a stale file from a previous run can't be
/// mistaken for a live daemon.
pub fn socket_path() -> PathBuf {
    std::env::temp_dir().join(format!("crowbar-{}.sock", std::process::id()))
}

pub async fn spawn<R: Runtime>(
    app: &AppHandle<R>,
    socket: PathBuf,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    // Remove any stale socket file so the daemon can bind cleanly.
    let _ = std::fs::remove_file(&socket);

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
    fn socket_path_is_pid_scoped_sock() {
        let p = socket_path();
        let name = p.file_name().unwrap().to_string_lossy().into_owned();
        assert!(name.starts_with("crowbar-"), "got {name}");
        assert!(name.ends_with(".sock"), "got {name}");
    }
}
