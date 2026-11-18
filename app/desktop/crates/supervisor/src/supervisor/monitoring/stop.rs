use std::process::ExitStatus;
use std::time::Duration;

use rustix::process::{Pid, Signal, kill_process};
use tokio::process::Child;

fn pid_of(id: Option<u32>) -> Option<Pid> {
    id.and_then(|id| i32::try_from(id).ok())
        .and_then(Pid::from_raw)
}

pub(crate) fn send(
    child: &Child,
    signal: Signal,
) {
    let Some(pid) = pid_of(child.id()) else {
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
mod tests {
    use super::*;

    #[test]
    fn only_a_live_process_id_names_a_pid_to_signal() {
        let cases = [
            ("already reaped", None, None),
            ("pid zero would signal our own group", Some(0), None),
            ("beyond the pid range", Some(u32::MAX), None),
            ("a process", Some(4242), Pid::from_raw(4242)),
        ];
        for (name, id, expected) in cases {
            assert_eq!(pid_of(id), expected, "{name}");
        }
    }
}
