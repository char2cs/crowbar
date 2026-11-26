pub(crate) mod adapters;
pub(crate) mod bus;
mod clock;
mod command;
pub(crate) mod log;
mod phase;
mod reply;
pub(crate) mod restart_budget;
mod startup;
mod verdict;

pub(crate) use bus::Bus;
pub(crate) use clock::Clock;
pub(crate) use command::{Command, next_command};
pub(crate) use log::DaemonLog;
pub(crate) use log::{MAX_LINE_BYTES, crash_report, crash_summary};
pub(crate) use phase::Phase;
pub(crate) use reply::reply;
pub(crate) use startup::Startup;
pub(crate) use verdict::Verdict;

use std::sync::Arc;

use crate::config::Config;
use crate::supervisor::supervision::adapters::system_clock;
use crate::supervisor::supervision::restart_budget::RestartBudget;

pub(crate) struct Supervision {
    pub(crate) config: Config,
    pub(crate) bus: Arc<Bus>,
    pub(crate) log: Option<Arc<DaemonLog>>,
    pub(crate) budget: RestartBudget,
    pub(crate) clock: Clock,
    pub(crate) launches: u32,
}

impl Supervision {
    pub(crate) fn new(
        config: Config,
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
            launches: 0,
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
    use crate::config::DaemonPaths;

    #[tokio::test]
    #[ignore = "real-infra: writes the real daemon log file"]
    async fn notes_go_to_the_daemon_log() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("logs").join("daemon.log");
        let paths = DaemonPaths {
            socket: dir.path().join("d.sock"),
            log: log.clone(),
        };
        let config = Config::new("crowbar-api".into(), paths, None, "test");
        let supervision = Supervision::new(config, Arc::new(Bus::default()));
        supervision.note("===", "daemon spawned");
        supervision.flush_log().await;
        assert_eq!(
            std::fs::read_to_string(log).unwrap(),
            "=== daemon spawned\n"
        );
    }
}
