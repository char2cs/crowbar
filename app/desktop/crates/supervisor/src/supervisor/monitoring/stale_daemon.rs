mod stale_daemon_event;

use rustix::process::Signal;
use tokio::process::Child;

use crate::supervisor::monitoring::stale_daemon::stale_daemon_event::StaleDaemonEvent;
use crate::supervisor::monitoring::stop::stop_with;
use crate::supervisor::supervision::Phase;
use crate::supervisor::supervision::Supervision;

pub(crate) async fn refuse(
    supervision: &Supervision,
    child: &mut Child,
    reason: String,
) -> Phase {
    crate::journal_sink::emit(&StaleDaemonEvent::Refused { reason: &reason });
    supervision.note("===", &reason);
    if let Err(error) = stop_with(
        child,
        Signal::TERM,
        supervision.config.timings.sigterm_grace,
    )
    .await
    {
        crate::journal_sink::emit(&StaleDaemonEvent::Unstopped { error: &error });
    }
    Phase::Recover { reason }
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
