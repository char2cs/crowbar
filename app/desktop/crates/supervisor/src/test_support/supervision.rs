use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use tokio::sync::broadcast;

use crate::config::Config;
use crate::config::DaemonPaths;
use crate::supervisor::DaemonEvent;
use crate::supervisor::supervision::Supervision;
use crate::supervisor::supervision::adapters::system_clock::system_clock;
use crate::supervisor::supervision::bus::Bus;
use crate::supervisor::supervision::restart_budget::RestartBudget;

pub(crate) fn supervision() -> Supervision {
    let paths = DaemonPaths {
        socket: "/nonexistent/d.sock".into(),
        log: "/nonexistent/daemon.log".into(),
    };
    let config = Config::new("crowbar-api".into(), paths, None, "test");
    let budget = RestartBudget::new(config.timings.restart_max, config.timings.restart_window);
    Supervision {
        config,
        bus: Arc::new(Bus::default()),
        log: None,
        budget,
        clock: system_clock(),
        launches: 0,
    }
}

pub(crate) fn missing_binary() -> Config {
    let paths = DaemonPaths {
        socket: PathBuf::from("/crowbar-test-missing/d.sock"),
        log: PathBuf::from("/crowbar-test-missing/d.log"),
    };
    let binary = PathBuf::from("/definitely/not/crowbar-api");
    let mut config = Config::new(binary, paths, None, "test");
    config.timings.respawn_base = Duration::from_millis(1);
    config
}

pub(crate) async fn until_recovering(events: &mut broadcast::Receiver<DaemonEvent>) {
    while !matches!(events.recv().await, Ok(DaemonEvent::Recovering { .. })) {}
}
