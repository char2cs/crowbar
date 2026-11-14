use std::path::Path;
use std::time::Duration;

use serde_json::Value;
use tokio::net::UnixStream;
use transport::local_get;

use crate::error::Error;

const HEALTH_BODY_LIMIT: usize = 1024 * 1024;

/// What `/v0/health` says about the daemon: serving, still preparing its home, or halted. A
/// daemon that is preparing or halted is alive on purpose and is never mistaken for a wedge.
#[derive(Debug, PartialEq, Eq)]
pub enum Health {
    /// The daemon serves, and says which process it is.
    Ready {
        /// The daemon's self-reported pid.
        pid: Option<u32>,
    },
    /// The daemon is preparing its home.
    Migrating,
    /// The daemon refuses to serve, and says why.
    Halted {
        /// The halt text, `data.detail.message`.
        message: String,
    },
}

/// Reads the `/v0/health` envelope. An absent status is an older daemon and an unknown one a
/// newer daemon; both are serving, so only the deep probe may judge them.
#[must_use]
pub fn parse_health(body: &[u8]) -> Health {
    let data = serde_json::from_slice::<Value>(body)
        .ok()
        .and_then(|v| v.get("data").cloned());
    let field = |name: &str| data.as_ref().and_then(|d| d.get(name));
    match field("status").and_then(Value::as_str) {
        Some("migrating") => Health::Migrating,
        Some("halted") => Health::Halted {
            message: field("detail")
                .and_then(|d| d.get("message"))
                .and_then(Value::as_str)
                .unwrap_or("the daemon halted without saying why")
                .to_owned(),
        },
        _ => Health::Ready {
            pid: field("pid")
                .and_then(Value::as_u64)
                .and_then(|pid| u32::try_from(pid).ok())
                .filter(|pid| *pid > 0),
        },
    }
}

pub(crate) async fn get(socket: &Path, target: &str, limit: usize) -> Result<Vec<u8>, String> {
    let stream = UnixStream::connect(socket)
        .await
        .map_err(|e| e.to_string())?;
    let response = local_get(stream, target, limit)
        .await
        .map_err(|e| e.to_string())?;
    if !response.status().is_success() {
        return Err(format!("{target} returned {}", response.status()));
    }
    Ok(response.into_body())
}

pub(crate) async fn check_health(socket: &Path) -> Result<Health, String> {
    get(socket, "/v0/health", HEALTH_BODY_LIMIT)
        .await
        .map(|body| parse_health(&body))
}

/// Polls until the daemon serves (resolving to its self-reported pid) or halts. A migrating
/// daemon is waited for without spending `attempts`; only unreachable or wrong answers do.
///
/// # Errors
/// [`Error::Halted`] when the daemon halts, [`Error::GaveUp`] when it never answers.
pub async fn wait_for_health(
    socket: &Path,
    attempts: u32,
    poll: Duration,
    probe_timeout: Duration,
) -> Result<Option<u32>, Error> {
    let mut failed = 0;
    loop {
        tokio::time::sleep(poll).await;
        match tokio::time::timeout(probe_timeout, check_health(socket)).await {
            Ok(Ok(Health::Ready { pid })) => return Ok(pid),
            Ok(Ok(Health::Halted { message })) => return Err(Error::Halted(message)),
            Ok(Ok(Health::Migrating)) => continue,
            _ => failed += 1,
        }
        if failed >= attempts {
            return Err(Error::GaveUp(attempts));
        }
    }
}

#[cfg(test)]
#[path = "health_tests.rs"]
mod tests;
