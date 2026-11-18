use std::io::{self, Write};
use std::path::Path;
use std::process::{Command, Stdio};
use std::time::Duration;

use supervisor::{Config, DaemonEvent, DaemonPaths, Timings};
use tempfile::TempDir;
use tokio::sync::broadcast::Receiver;
use tokio::sync::broadcast::error::RecvError;

const EVENT_BUDGET: Duration = Duration::from_secs(30);

pub(crate) fn fast_timings() -> Timings {
    Timings {
        health_poll: Duration::from_millis(10),
        health_attempts: 100,
        health_probe_timeout: Duration::from_secs(2),
        watchdog_interval: Duration::from_secs(3600),
        respawn_base: Duration::from_millis(5),
        respawn_cap: Duration::from_millis(20),
        sigterm_grace: Duration::from_secs(5),
        ..Timings::default()
    }
}

/// A supervisor config whose `crowbar-api` is a script that runs the fake daemon in `mode`. The
/// returned directory holds the socket, script and log and is removed when it is dropped.
pub(crate) fn config(
    mode: &str,
    timings: Timings,
) -> io::Result<(Config, TempDir)> {
    let dir = tempfile::Builder::new().prefix("cb-sup-").tempdir()?;
    let script = dir.path().join("crowbar-api");
    let fake = env!("CARGO_BIN_EXE_fake_daemon");
    let body = format!("#!/bin/sh\nFAKE_DAEMON_MODE={mode} exec {fake} \"$@\"\n");
    write_script(&script, &body)?;
    let paths = DaemonPaths {
        socket: dir.path().join("d.sock"),
        log: dir.path().join("logs").join("daemon.log"),
    };
    let mut config = Config::new(script, paths, None, "1.2.3-test");
    config.timings = timings;
    Ok((config, dir))
}

const WRITE_EXECUTABLE: &str = "cat > \"$1\" && chmod 755 \"$1\"";

/// Writes `body` to `path` as an executable script through a short-lived `/bin/sh` child, so no
/// descriptor of the script ever exists in the test process: a sibling test thread that forks
/// meanwhile cannot inherit a writer of it, and executing it never fails with `ETXTBSY`.
fn write_script(
    path: &Path,
    body: &str,
) -> io::Result<()> {
    let mut writer = Command::new("/bin/sh")
        .args(["-c", WRITE_EXECUTABLE, "sh"])
        .arg(path)
        .stdin(Stdio::piped())
        .spawn()?;
    let mut input = writer
        .stdin
        .take()
        .ok_or_else(|| io::Error::other("no stdin pipe"))?;
    input.write_all(body.as_bytes())?;
    drop(input);
    let status = writer.wait()?;
    if !status.success() {
        return Err(io::Error::other(format!(
            "writing {} failed: {status}",
            path.display()
        )));
    }
    Ok(())
}

async fn first_matching(
    events: &mut Receiver<DaemonEvent>,
    wanted: impl Fn(&DaemonEvent) -> bool,
) -> Result<DaemonEvent, RecvError> {
    let mut event = events.recv().await?;
    while !wanted(&event) {
        event = events.recv().await?;
    }
    Ok(event)
}

/// The next event `wanted` accepts, or an error when the stream closes or `EVENT_BUDGET` runs out.
pub(crate) async fn next_matching(
    events: &mut Receiver<DaemonEvent>,
    wanted: impl Fn(&DaemonEvent) -> bool,
) -> Result<DaemonEvent, Box<dyn std::error::Error>> {
    Ok(tokio::time::timeout(EVENT_BUDGET, first_matching(events, wanted)).await??)
}
