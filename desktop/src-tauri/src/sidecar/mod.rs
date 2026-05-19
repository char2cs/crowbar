use std::path::PathBuf;
use std::sync::Mutex;
use std::time::Duration;

use bytes::Bytes;
use http_body_util::Empty;
use hyper_util::client::legacy::Client;
use hyper_util::rt::TokioExecutor;
use hyperlocal::{UnixConnector, Uri as UnixUri};
use tauri::{AppHandle, Manager, Runtime};
use tauri_plugin_shell::{process::CommandChild, ShellExt};

/// Holds the child process handle for the crowbar-api sidecar so it can be
/// killed cleanly when the Tauri window closes.
pub struct SidecarHandle {
    pub child: Mutex<Option<CommandChild>>,
}

impl SidecarHandle {
    pub fn new() -> Self {
        Self {
            child: Mutex::new(None),
        }
    }
}

pub fn socket_path() -> PathBuf {
    let home = dirs::home_dir().expect("no home directory");
    home.join(".crowbar").join("crowbar.sock")
}

pub async fn spawn<R: Runtime>(
    app: &AppHandle<R>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let sidecar = app
        .shell()
        .sidecar("crowbar-api")?
        .args(["serve", "--host", "unix://"]);

    let (_rx, child) = sidecar.spawn()?;

    // Store child in managed state so it can be killed on window close.
    app.state::<SidecarHandle>()
        .child
        .lock()
        .unwrap()
        .replace(child);

    wait_for_health(30).await?;
    log::info!("crowbar daemon is ready");
    Ok(())
}

async fn wait_for_health(
    attempts: u32,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    for i in 0..attempts {
        tokio::time::sleep(Duration::from_millis(200)).await;

        if check_health().await.is_ok() {
            return Ok(());
        }

        if i == attempts - 1 {
            return Err("daemon did not become healthy within 6s".into());
        }
    }
    Ok(())
}

async fn check_health() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let socket = socket_path();
    if !socket.exists() {
        return Err("socket not found".into());
    }

    let url: hyper::Uri = UnixUri::new(&socket, "/api/v0/health").into();
    let client: Client<UnixConnector, Empty<Bytes>> =
        Client::builder(TokioExecutor::new()).build(UnixConnector);

    let resp = client.get(url).await?;
    if resp.status().is_success() {
        Ok(())
    } else {
        Err(format!("health check returned {}", resp.status()).into())
    }
}

#[cfg(test)]
mod tests {
    use super::socket_path;

    #[test]
    fn socket_path_ends_with_sock() {
        let p = socket_path();
        assert!(
            p.ends_with("crowbar.sock"),
            "expected crowbar.sock, got {:?}",
            p
        );
    }
}
