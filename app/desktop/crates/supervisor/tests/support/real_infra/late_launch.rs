use std::time::Duration;

use supervisor::{DaemonEvent, Supervisor, Timings};
use tempfile::TempDir;
use tokio::net::UnixListener;
use tokio::sync::broadcast::Receiver;

use crate::journal_lines::listen;
use crate::support::{config, next_matching, within_budget};

const STARTUP_POLLS: u32 = 3;

pub(crate) struct Launch {
    pub(crate) supervisor: Supervisor,
    pub(crate) events: Receiver<DaemonEvent>,
    listening: UnixListener,
    home: TempDir,
}

pub(crate) async fn launch_until_startup_is_spent(
    timings: Timings
) -> Result<Launch, Box<dyn std::error::Error>> {
    let (cfg, home) = config(
        "late",
        Timings {
            health_attempts: STARTUP_POLLS,
            ..timings
        },
    )?;
    let listening = UnixListener::bind(home.path().join("d.sock.up"))?;
    let (mut lines, _journal) = listen();
    let supervisor = Supervisor::start(cfg);
    let events = supervisor.events();
    within_budget(async {
        while !lines.recv().await?.contains("daemon_unserved") {}
        Some(())
    })
    .await?
    .ok_or("the journal closed before startup was given up")?;
    Ok(Launch {
        supervisor,
        events,
        listening,
        home,
    })
}

impl Launch {
    pub(crate) async fn let_serve(&self) -> Result<(), Box<dyn std::error::Error>> {
        within_budget(self.listening.accept()).await??;
        std::fs::write(self.home.path().join("d.sock.serve"), b"")?;
        Ok(())
    }
}

pub(crate) async fn heard_until(
    events: &mut Receiver<DaemonEvent>,
    last: impl Fn(&DaemonEvent) -> bool,
) -> Result<Vec<DaemonEvent>, Box<dyn std::error::Error>> {
    let mut heard = Vec::new();
    while !heard.last().is_some_and(&last) {
        heard.push(next_matching(events, |_| true).await?);
    }
    Ok(heard)
}

pub(crate) fn probing(failure_threshold: u32) -> Timings {
    Timings {
        watchdog_interval: Duration::from_millis(20),
        probe_timeout: Duration::from_millis(300),
        grace_probe_timeout: Duration::from_millis(300),
        failure_threshold,
        dump_timeout: Duration::from_millis(100),
        sigquit_grace: Duration::from_secs(5),
        ..crate::support::fast_timings()
    }
}
