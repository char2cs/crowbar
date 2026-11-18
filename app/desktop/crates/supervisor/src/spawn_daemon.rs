use std::path::Path;
use std::process::Stdio;

use tokio::process::{Child, Command};

use crate::adapters::private_dir;
use crate::error::Error;
use crate::output_pump::capture;
use crate::supervision::Supervision;

fn make_socket_dir(dir: &Path) -> Result<(), Error> {
    private_dir(dir).map_err(|source| Error::Spawn {
        path: dir.to_path_buf(),
        source,
    })
}

fn output_sink(supervision: &Supervision) -> Stdio {
    match supervision.log {
        Some(_) => Stdio::piped(),
        None => Stdio::null(),
    }
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
        .stdout(output_sink(supervision))
        .stderr(output_sink(supervision))
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
    #[ignore = "real-infra: creates a real socket directory and reads its 0700 mode"]
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

    #[tokio::test]
    #[ignore = "real-infra: runs a real script that floods stdout and stderr"]
    async fn a_daemon_whose_log_cannot_be_opened_never_blocks_on_a_full_pipe() {
        let dir = tempfile::tempdir().unwrap();
        let script = dir.path().join("noisy.sh");
        std::fs::write(
            &script,
            "#!/bin/sh\nhead -c 1048576 /dev/zero >&2\nhead -c 1048576 /dev/zero\n",
        )
        .unwrap();
        std::fs::set_permissions(&script, std::fs::Permissions::from_mode(0o755)).unwrap();
        let paths = DaemonPaths {
            socket: dir.path().join("run").join("d.sock"),
            log: dir.path().to_path_buf(),
        };
        let config = SupervisorConfig::new(script, paths, None, "test");
        let supervision = Supervision::new(config, Arc::new(Bus::default()));
        assert!(supervision.log.is_none(), "the log is not openable");
        let mut child = spawn_daemon(&supervision).unwrap();
        let status = tokio::time::timeout(std::time::Duration::from_secs(10), child.wait())
            .await
            .expect("the daemon is not wedged on its output")
            .unwrap();
        assert!(status.success());
    }
}
