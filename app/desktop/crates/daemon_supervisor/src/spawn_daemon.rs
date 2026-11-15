use std::os::unix::fs::DirBuilderExt;
use std::path::Path;
use std::process::Stdio;

use tokio::process::{Child, Command};

use crate::error::Error;
use crate::output_pump::capture;
use crate::supervision::Supervision;

const SOCKET_DIR_MODE: u32 = 0o700;

fn make_socket_dir(dir: &Path) -> Result<(), Error> {
    std::fs::DirBuilder::new()
        .recursive(true)
        .mode(SOCKET_DIR_MODE)
        .create(dir)
        .map_err(|source| Error::Spawn {
            path: dir.to_path_buf(),
            source,
        })
}

pub(crate) fn spawn_daemon(supervision: &Supervision) -> Result<Child, Error> {
    let config = &supervision.config;
    config
        .paths
        .socket
        .parent()
        .map(make_socket_dir)
        .transpose()?;
    let host = format!("unix://{}", config.paths.socket.display());
    let mut command = Command::new(&config.binary);
    command
        .args(["serve", "--host", &host])
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true);
    if let Some(home) = &config.home {
        command.env("CROWBAR_HOME", home);
    }
    let mut child = command.spawn().map_err(|source| Error::Spawn {
        path: config.binary.clone(),
        source,
    })?;
    if let Some(log) = &supervision.log {
        capture(&mut child, log);
    }
    Ok(child)
}

#[cfg(test)]
mod tests {
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
        let config =
            SupervisorConfig::new("/definitely/not/crowbar-api".into(), paths, None, "test");
        let supervision = Supervision::new(config, Arc::new(Bus::default()));
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
}
