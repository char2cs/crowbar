use super::*;
use crate::daemon_paths::DaemonPaths;

fn config(tag: &str) -> SupervisorConfig {
    let dir = std::env::temp_dir().join(format!("cb-supn-{tag}-{}", std::process::id()));
    SupervisorConfig::new(
        "crowbar-api".into(),
        DaemonPaths {
            socket: dir.join("d.sock"),
            log: dir.join("logs").join("daemon.log"),
        },
        None,
    )
}

#[test]
fn notes_go_to_the_daemon_log() {
    let config = config("note");
    let log = config.paths.log.clone();
    let supervision = Supervision::new(config, Arc::new(Bus::new()));
    supervision.note("===", "daemon spawned");
    assert_eq!(
        std::fs::read_to_string(log).unwrap(),
        "=== daemon spawned\n"
    );
}

#[test]
fn a_log_that_cannot_be_opened_does_not_stop_supervision() {
    let mut config = config("nolog");
    config.paths.log = "/proc/definitely/not/writable/daemon.log".into();
    let supervision = Supervision::new(config, Arc::new(Bus::new()));
    assert!(supervision.log.is_none());
    supervision.note("===", "ignored");
}
