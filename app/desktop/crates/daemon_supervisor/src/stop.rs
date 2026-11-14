use std::process::ExitStatus;
use std::time::Duration;

use rustix::process::{Pid, Signal, kill_process};
use tokio::process::Child;

pub(crate) fn send(child: &Child, signal: Signal) {
    let pid = child
        .id()
        .and_then(|id| i32::try_from(id).ok())
        .and_then(Pid::from_raw);
    let Some(pid) = pid else {
        return;
    };
    if let Err(e) = kill_process(pid, signal) {
        tracing::debug!("signal {signal:?} to daemon {pid:?}: {e}");
    }
}

pub(crate) async fn stop_with(
    child: &mut Child,
    first: Signal,
    grace: Duration,
) -> std::io::Result<ExitStatus> {
    send(child, first);
    if let Ok(status) = tokio::time::timeout(grace, child.wait()).await {
        return status;
    }
    child.kill().await?;
    child.wait().await
}

#[cfg(test)]
#[path = "stop_tests.rs"]
mod tests;
