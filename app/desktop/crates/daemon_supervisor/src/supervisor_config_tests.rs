use std::path::PathBuf;

use super::*;

#[test]
fn a_new_config_runs_with_production_timings() {
    let paths = DaemonPaths {
        socket: PathBuf::from("/s.sock"),
        log: PathBuf::from("/l.log"),
    };
    let config = SupervisorConfig::new(
        PathBuf::from("/bin/crowbar-api"),
        paths,
        Some(PathBuf::from("/home")),
    );
    assert_eq!(config.timings.restart_max, Timings::default().restart_max);
    assert_eq!(config.home, Some(PathBuf::from("/home")));
}
