use std::os::unix::fs::PermissionsExt;

use super::*;
use crate::bus::Bus;
use crate::daemon_paths::DaemonPaths;
use crate::supervisor_config::SupervisorConfig;
use std::sync::Arc;

#[tokio::test]
async fn a_binary_that_cannot_run_is_a_typed_spawn_error() {
    let dir = tempfile::tempdir().unwrap();
    let paths = DaemonPaths {
        socket: dir.path().join("run").join("d.sock"),
        log: dir.path().join("d.log"),
    };
    let config = SupervisorConfig::new("/definitely/not/crowbar-api".into(), paths, None, "test");
    let supervision = Supervision::new(config, Arc::new(Bus::new()));
    let err = spawn_daemon(&supervision).unwrap_err();
    assert!(matches!(err, Error::Spawn { .. }), "{err}");
    let run = dir.path().join("run");
    assert!(run.is_dir(), "the socket's directory is made");
    assert_eq!(
        std::fs::metadata(run).unwrap().permissions().mode() & 0o777,
        0o700,
        "private"
    );
}
