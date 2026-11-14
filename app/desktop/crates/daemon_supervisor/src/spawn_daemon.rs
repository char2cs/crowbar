use std::process::Stdio;

use tokio::process::{Child, Command};

use crate::error::Error;
use crate::output_pump::capture;
use crate::supervision::Supervision;

pub(crate) fn spawn_daemon(supervision: &Supervision) -> Result<Child, Error> {
    let config = &supervision.config;
    if let Some(dir) = config.paths.socket.parent() {
        std::fs::create_dir_all(dir).map_err(|source| Error::Spawn {
            path: dir.to_path_buf(),
            source,
        })?;
    }
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
#[path = "spawn_daemon_tests.rs"]
mod tests;
