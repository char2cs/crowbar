use rustix::process::Signal;
use tokio::process::Child;

use crate::phase::Phase;
use crate::stop::stop_with;
use crate::supervision::Supervision;

pub(crate) async fn refuse(
    supervision: &Supervision,
    child: &mut Child,
    reason: String,
) -> Phase {
    tracing::error!("{reason}");
    supervision.note("===", &reason);
    if let Err(error) = stop_with(
        child,
        Signal::TERM,
        supervision.config.timings.sigterm_grace,
    )
    .await
    {
        tracing::warn!("stopping the daemon that lost the socket: {error}");
    }
    Phase::Recover { reason }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::bus::Bus;
    use crate::daemon_paths::DaemonPaths;
    use crate::supervisor_config::SupervisorConfig;

    #[tokio::test]
    async fn a_daemon_that_is_not_the_one_we_started_is_stopped_and_recovered_from() {
        let dir = tempfile::tempdir().unwrap();
        let paths = DaemonPaths {
            socket: dir.path().join("d.sock"),
            log: dir.path().join("d.log"),
        };
        let config = SupervisorConfig::new("sleep".into(), paths, None, "test");
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
