use std::net::TcpListener;
use std::sync::Mutex;
use std::time::Duration;

use bytes::Bytes;
use http_body_util::Empty;
use hyper_util::client::legacy::connect::HttpConnector;
use hyper_util::client::legacy::Client;
use hyper_util::rt::TokioExecutor;
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

/// Reserve a free localhost TCP port by binding to port 0 and reading back the
/// assigned port. The listener is dropped immediately so the sidecar can bind
/// it; the brief gap is acceptable for a single desktop launch.
pub fn pick_free_port() -> u16 {
    TcpListener::bind("127.0.0.1:0")
        .expect("failed to reserve a local port")
        .local_addr()
        .expect("failed to read reserved port")
        .port()
}

pub async fn spawn<R: Runtime>(
    app: &AppHandle<R>,
    port: u16,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let host = format!("tcp://127.0.0.1:{port}");
    let sidecar = app
        .shell()
        .sidecar("crowbar-api")?
        .args(["serve", "--host", &host]);

    let (_rx, child) = sidecar.spawn()?;

    // Store child in managed state so it can be killed on window close.
    app.state::<SidecarHandle>()
        .child
        .lock()
        .unwrap()
        .replace(child);

    wait_for_health(port, 30).await?;
    log::info!("crowbar daemon is ready on 127.0.0.1:{port}");
    Ok(())
}

async fn wait_for_health(
    port: u16,
    attempts: u32,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    for i in 0..attempts {
        tokio::time::sleep(Duration::from_millis(200)).await;

        if check_health(port).await.is_ok() {
            return Ok(());
        }

        if i == attempts - 1 {
            return Err("daemon did not become healthy within 6s".into());
        }
    }
    Ok(())
}

async fn check_health(
    port: u16,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let url: hyper::Uri = format!("http://127.0.0.1:{port}/v0/health").parse()?;
    let client: Client<HttpConnector, Empty<Bytes>> =
        Client::builder(TokioExecutor::new()).build(HttpConnector::new());

    let resp = client.get(url).await?;
    if resp.status().is_success() {
        Ok(())
    } else {
        Err(format!("health check returned {}", resp.status()).into())
    }
}

#[cfg(test)]
mod tests {
    use super::pick_free_port;

    #[test]
    fn pick_free_port_returns_nonzero() {
        let p = pick_free_port();
        assert!(p > 0, "expected a nonzero port, got {p}");
    }
}
