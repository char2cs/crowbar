use std::sync::Arc;

use crate::supervisor::supervision::Supervision;
use crate::supervisor::supervision::adapters::system_clock::system_clock;
use crate::supervisor::supervision::bus::Bus;
use crate::supervisor::supervision::restart_budget::RestartBudget;
use crate::supervisor_config::DaemonPaths;
use crate::supervisor_config::SupervisorConfig;

pub(crate) fn supervision() -> Supervision {
    let paths = DaemonPaths {
        socket: "/nonexistent/d.sock".into(),
        log: "/nonexistent/daemon.log".into(),
    };
    let config = SupervisorConfig::new("crowbar-api".into(), paths, None, "test");
    let budget = RestartBudget::new(config.timings.restart_max, config.timings.restart_window);
    Supervision {
        config,
        bus: Arc::new(Bus::default()),
        log: None,
        budget,
        clock: system_clock(),
    }
}
