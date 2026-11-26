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

/// Starts a `late` daemon and returns once every startup poll has been refused and the daemon
/// listens.
///
/// The test holds the daemon's socket until the daemon renames its own onto the path, and
/// refuses the polls that reach it first, so every poll is refused by one or the other.
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
    let early = UnixListener::bind(&cfg.paths.socket)?;
    let spent = UnixListener::bind(home.path().join("d.sock.spent"))?;
    let up = UnixListener::bind(home.path().join("d.sock.up"))?;
    let supervisor = Supervisor::start(cfg);
    let events = supervisor.events();
    tokio::time::timeout(EVENT_BUDGET, refuse_startup(&early, &spent, &up))
        .await
        .map_err(|elapsed| format!("startup polls not all refused, or no daemon: {elapsed}"))??;
    Ok(Launch {
        supervisor,
        events,
        home,
    })
}

async fn refuse_startup(
    early: &UnixListener,
    spent: &UnixListener,
    up: &UnixListener,
) -> std::io::Result<()> {
    for _ in 0..STARTUP_POLLS {
        refuse_one(early, spent).await?;
    }
    up.accept().await.map(drop)
}

async fn refuse_one(
    early: &UnixListener,
    spent: &UnixListener,
) -> std::io::Result<()> {
    tokio::select! {
        poll = early.accept() => poll.map(drop),
        told = spent.accept() => told.map(drop),
    }
}

impl Launch {
    /// Lets the daemon serve from its next health request on.
    pub(crate) fn let_serve(&self) -> std::io::Result<()> {
        std::fs::write(self.home.path().join("d.sock.serve"), b"")
    }
}

/// Every event up to and including the first one `last` accepts, all within `EVENT_BUDGET`.
pub(crate) async fn heard_until(
    events: &mut Receiver<DaemonEvent>,
    last: impl Fn(&DaemonEvent) -> bool,
) -> Result<Vec<DaemonEvent>, Box<dyn std::error::Error>> {
    let mut heard = Vec::new();
    let listened = tokio::time::timeout(EVENT_BUDGET, hear_until(events, &last, &mut heard));
    match listened.await {
        Ok(done) => done.map(|()| heard),
        Err(elapsed) => Err(format!("{elapsed}, heard {heard:?}").into()),
    }
}

async fn hear_until(
    events: &mut Receiver<DaemonEvent>,
    last: &impl Fn(&DaemonEvent) -> bool,
    heard: &mut Vec<DaemonEvent>,
) -> Result<(), Box<dyn std::error::Error>> {
    while !heard.last().is_some_and(last) {
        heard.push(next_matching(events, |_| true).await?);
    }
    Ok(())
}

/// Stops the supervisor, within `EVENT_BUDGET`.
pub(crate) async fn stopped(supervisor: Supervisor) -> Result<(), Box<dyn std::error::Error>> {
    tokio::time::timeout(EVENT_BUDGET, supervisor.stop())
        .await
        .map_err(|elapsed| format!("the supervisor did not stop: {elapsed}").into())
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
