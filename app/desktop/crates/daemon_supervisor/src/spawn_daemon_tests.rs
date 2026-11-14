use super::*;
use crate::bus::Bus;
use crate::daemon_paths::DaemonPaths;
use crate::supervisor_config::SupervisorConfig;
use std::sync::Arc;

#[tokio::test]
async fn a_binary_that_cannot_run_is_a_typed_spawn_error() {
    let dir = std::env::temp_dir().join(format!("cb-spawn-{}", std::process::id()));
    let paths = DaemonPaths {
        socket: dir.join("run").join("d.sock"),
        log: dir.join("d.log"),
    };
    let config = SupervisorConfig::new("/definitely/not/crowbar-api".into(), paths, None);
    let supervision = Supervision::new(config, Arc::new(Bus::new()));
    let err = spawn_daemon(&supervision).unwrap_err();
    assert!(matches!(err, Error::Spawn { .. }), "{err}");
    assert!(dir.join("run").is_dir(), "the socket's directory is made");
}
