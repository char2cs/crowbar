use std::sync::Arc;

use crate::bus::Bus;
use crate::daemon_log::DaemonLog;
use crate::restart_budget::RestartBudget;
use crate::supervisor_config::SupervisorConfig;

pub(crate) struct Supervision {
    pub(crate) config: SupervisorConfig,
    pub(crate) bus: Arc<Bus>,
    pub(crate) log: Option<Arc<DaemonLog>>,
    pub(crate) budget: RestartBudget,
}

impl Supervision {
    pub(crate) fn new(config: SupervisorConfig, bus: Arc<Bus>) -> Self {
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
        }
    }

    pub(crate) fn note(&self, prefix: &str, text: &str) {
        if let Some(log) = &self.log {
            log.line(prefix, text);
        }
    }
}

#[cfg(test)]
#[path = "supervision_tests.rs"]
mod tests;
