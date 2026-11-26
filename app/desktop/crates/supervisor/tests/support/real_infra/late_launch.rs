use std::time::Duration;

use supervisor::{DaemonEvent, Supervisor, Timings};
use tempfile::TempDir;
use tokio::net::UnixListener;
use tokio::sync::broadcast::Receiver;

use crate::support::{EVENT_BUDGET, config, next_matching};

const STARTUP_POLLS: u32 = 3;

/// A `late` fake daemon whose startup budget is already spent: it runs, unserved.
pub(crate) struct Launch {
    pub(crate) supervisor: Supervisor,
    pub(crate) events: Receiver<DaemonEvent>,
    home: TempDir,
}

/// Starts a `late` daemon and returns once every startup poll has been refused.
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
    let spent = UnixListener::bind(home.path().join("d.sock.spent"))?;
    let supervisor = Supervisor::start(cfg);
    let events = supervisor.events();
    let mut events_heard = supervisor.events();
    if let Err((polled, missed)) = refused_polls(&spent).await {
        return Err(missed_report(&missed, polled, &mut events_heard, home.path()).into());
    }
    Ok(Launch {
        supervisor,
        events,
        home,
    })
}

async fn refused_polls(spent: &UnixListener) -> Result<(), (u32, String)> {
    for polled in 0..STARTUP_POLLS {
        refused_poll(spent).await.map_err(|missed| (polled, missed))?;
    }
    Ok(())
}

async fn refused_poll(spent: &UnixListener) -> Result<(), String> {
    let dialled = tokio::time::timeout(EVENT_BUDGET, spent.accept()).await;
    dialled
        .map_err(|_| format!("no refused poll within {EVENT_BUDGET:?}"))?
        .map(drop)
        .map_err(|error| error.to_string())
}

fn missed_report(
    missed: &str,
    polled: u32,
    events: &mut Receiver<DaemonEvent>,
    home: &std::path::Path,
) -> String {
    let log = std::fs::read_to_string(home.join("logs").join("daemon.log"));
    let heard = drain(events);
    format!(
        "{missed} after {polled} of {STARTUP_POLLS}; home {}; events {heard:?}; log {log:?}",
        home.display()
    )
}

fn drain(events: &mut Receiver<DaemonEvent>) -> Vec<DaemonEvent> {
    std::iter::from_fn(|| events.try_recv().ok()).collect()
}

impl Launch {
    /// Lets the daemon serve from its next health request on.
    pub(crate) fn let_serve(&self) -> std::io::Result<()> {
        std::fs::write(self.home.path().join("d.sock.serve"), b"")
    }
}

/// Every event up to and including the first one `last` accepts.
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

/// Watchdog timings that probe every 20ms and kill after `failure_threshold` silent probes.
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
