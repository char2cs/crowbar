use std::sync::Arc;

use crate::adapters::system_clock;
use crate::bus::Bus;
use crate::clock::Clock;
use crate::daemon_log::DaemonLog;
use crate::restart_budget::RestartBudget;
use crate::supervisor_config::SupervisorConfig;

pub(crate) struct Supervision {
    pub(crate) config: SupervisorConfig,
    pub(crate) bus: Arc<Bus>,
    pub(crate) log: Option<Arc<DaemonLog>>,
    pub(crate) budget: RestartBudget,
    pub(crate) clock: Clock,
}

impl Supervision {
    pub(crate) fn new(
        config: SupervisorConfig,
        bus: Arc<Bus>,
    ) -> Self {
        let log = match DaemonLog::open(&config.paths.log, config.timings.log_max_bytes) {
            Ok(log) => Some(Arc::new(log)),
            Err(e) => {
                tracing::error!("{e}");
                None
            },
        };
        let budget = RestartBudget::new(config.timings.restart_max, config.timings.restart_window);
        Self {
            config,
            bus,
            log,
            budget,
            clock: system_clock(),
        }
    }

    pub(crate) fn note(
        &self,
        prefix: &str,
        text: &str,
    ) {
        if let Some(log) = &self.log {
            log.line(prefix, text);
        }
    }

    pub(crate) async fn flush_log(&self) {
        if let Some(log) = &self.log {
            log.flush().await;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::daemon_paths::DaemonPaths;

    fn config(dir: &std::path::Path) -> SupervisorConfig {
        SupervisorConfig::new(
            "crowbar-api".into(),
            DaemonPaths {
                socket: dir.join("d.sock"),
                log: dir.join("logs").join("daemon.log"),
            },
            None,
            "test",
        )
    }

    #[tokio::test]
    async fn notes_go_to_the_daemon_log() {
        let dir = tempfile::tempdir().unwrap();
        let config = config(dir.path());
        let log = config.paths.log.clone();
        let supervision = Supervision::new(config, Arc::new(Bus::default()));
        supervision.note("===", "daemon spawned");
        supervision.flush_log().await;
        assert_eq!(
            std::fs::read_to_string(log).unwrap(),
            "=== daemon spawned\n"
        );
    }

    #[test]
    fn a_log_that_cannot_be_opened_does_not_stop_supervision() {
        let mut config = config(std::path::Path::new("/nonexistent"));
        config.paths.log = "/proc/definitely/not/writable/daemon.log".into();
        let supervision = Supervision::new(config, Arc::new(Bus::default()));
        assert!(supervision.log.is_none());
        supervision.note("===", "ignored");
    }
}
