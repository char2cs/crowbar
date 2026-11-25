use rustix::process::Signal;
use tokio::process::Child;

use crate::supervisor::monitoring::stop::stop_with;
use crate::supervisor::supervision::Phase;
use crate::supervisor::supervision::Supervision;

pub(crate) async fn refuse(
    supervision: &Supervision,
    child: &mut Child,
    reason: String,
) -> Phase {
    log_refusal(&reason);
    supervision.note("===", &reason);
    stop_stale(supervision, child).await;
    Phase::Recover { reason }
}

async fn stop_stale(
    supervision: &Supervision,
    child: &mut Child,
) {
    let stopped = stop_with(
        child,
        Signal::TERM,
        supervision.config.timings.sigterm_grace,
    )
    .await;
    if let Err(error) = stopped {
        warn_unstopped(&error);
    }
}

fn log_refusal(reason: &str) {
    tracing::error!("{reason}");
}

fn warn_unstopped(error: &std::io::Error) {
    tracing::warn!("stopping the daemon that lost the socket: {error}");
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::config::Config;
    use crate::config::DaemonPaths;
    use crate::supervisor::supervision::Bus;

    #[tokio::test]
    #[ignore = "real-infra: stops a real child process"]
    async fn a_daemon_that_is_not_the_one_we_started_is_stopped_and_recovered_from() {
        let dir = tempfile::tempdir().unwrap();
        let paths = DaemonPaths {
            socket: dir.path().join("d.sock"),
            log: dir.path().join("d.log"),
        };
        let config = Config::new("sleep".into(), paths, None, "test");
        let supervision = Supervision::new(config, Arc::new(Bus::default()));
        let mut child = tokio::process::Command::new("sleep")
            .arg("60")
            .kill_on_drop(true)
            .spawn()
            .unwrap();
        let phase = refuse(&supervision, &mut child, "stale daemon".to_owned()).await;
        assert!(matches!(phase, Phase::Recover { reason } if reason == "stale daemon"));
        assert!(child.try_wait().unwrap().is_some(), "the child was stopped");
    }
}
