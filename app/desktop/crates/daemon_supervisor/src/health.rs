use std::path::Path;

use serde_json::Value;

use crate::announced::Announced;
use crate::daemon_get::get;
use crate::dial::Dial;
use crate::error::Error;
use crate::migration_progress::MigrationProgress;
use crate::socket_dial::SocketDial;
use crate::timings::Timings;

const HEALTH_BODY_LIMIT: usize = 1024 * 1024;
const HEALTH_TARGET: &str = "/v0/health";

/// What `/v0/health` says about the daemon: serving, still preparing its home, or halted. A
/// daemon that is preparing or halted is alive on purpose and is never mistaken for a wedge.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Health {
    /// The daemon serves, and says which process it is.
    Ready {
        /// The daemon's self-reported pid.
        pid: Option<u32>,
    },
    /// The daemon is preparing its home.
    Migrating {
        /// The current step, `None` before the first one.
        progress: Option<MigrationProgress>,
    },
    /// The daemon refuses to serve, and says why.
    Halted {
        /// The machine-readable halt reason, `data.detail.reason`.
        reason: Option<String>,
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
    let detail_text = |name: &str| {
        field("detail")
            .and_then(|d| d.get(name))
            .and_then(Value::as_str)
            .map(str::to_owned)
    };
    match field("status").and_then(Value::as_str) {
        Some("migrating") => Health::Migrating {
            progress: field("detail").and_then(MigrationProgress::parse),
        },
        Some("halted") => Health::Halted {
            reason: detail_text("reason"),
            message: detail_text("message")
                .unwrap_or_else(|| "the daemon halted without saying why".to_owned()),
        },
        _ => Health::Ready {
            pid: field("pid")
                .and_then(Value::as_u64)
                .and_then(|pid| u32::try_from(pid).ok())
                .filter(|pid| *pid > 0),
        },
    }
}

pub(crate) async fn check_health<D: Dial>(dial: &D) -> Result<Health, Error> {
    get(dial, HEALTH_TARGET, HEALTH_BODY_LIMIT)
        .await
        .map(|body| parse_health(&body))
}

/// Polls until the daemon serves (resolving to its self-reported pid) or halts. A migrating
/// daemon is waited for without spending attempts, and `on_migrating` hears every change of
/// its progress; only unreachable or wrong answers count against `timings.health_attempts`.
///
/// # Errors
/// [`Error::Halted`] when the daemon halts, [`Error::GaveUp`] when it never answers.
pub async fn wait_for_health(
    socket: &Path,
    timings: &Timings,
    on_migrating: impl FnMut(Option<&MigrationProgress>) + Send,
) -> Result<Option<u32>, Error> {
    wait_with(&SocketDial::new(socket), timings, on_migrating).await
}

pub(crate) async fn wait_with<D: Dial>(
    dial: &D,
    timings: &Timings,
    mut on_migrating: impl FnMut(Option<&MigrationProgress>) + Send,
) -> Result<Option<u32>, Error> {
    let mut failed = 0;
    let mut announce = Announced::default();
    loop {
        tokio::time::sleep(timings.health_poll).await;
        match tokio::time::timeout(timings.health_probe_timeout, check_health(dial)).await {
            Ok(Ok(Health::Ready { pid })) => return Ok(pid),
            Ok(Ok(Health::Halted { reason, message })) => {
                return Err(Error::Halted { reason, message });
            },
            Ok(Ok(Health::Migrating { progress })) => {
                announce.report(progress, &mut on_migrating);
                continue;
            },
            Ok(Err(error)) => tracing::debug!("daemon health: {error}"),
            Err(_) => tracing::debug!(
                "daemon health: no answer in {:?}",
                timings.health_probe_timeout
            ),
        }
        failed += 1;
        if failed >= timings.health_attempts {
            return Err(Error::GaveUp(timings.health_attempts));
        }
    }
}

#[cfg(test)]
#[path = "health_tests.rs"]
mod tests;
