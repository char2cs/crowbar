use std::sync::Arc;

use tokio::sync::{broadcast, mpsc, oneshot, watch};
use tokio::task::JoinHandle;

use crate::bus::Bus;
use crate::command::Command;
use crate::daemon_event::DaemonEvent;
use crate::error::Error;
use crate::supervise::supervise;
use crate::supervision::Supervision;
use crate::supervisor_config::SupervisorConfig;

/// Handle to the running supervision task. Dropping it without [`Supervisor::stop`] aborts the
/// task, which kills the daemon child (it is spawned with `kill_on_drop`).
#[derive(Debug)]
pub struct Supervisor {
    commands: mpsc::Sender<Command>,
    bus: Arc<Bus>,
    task: JoinHandle<()>,
}

impl Supervisor {
    /// Starts the daemon and supervises it until [`Supervisor::stop`].
    #[must_use]
    pub fn start(config: SupervisorConfig) -> Self {
        let bus = Arc::new(Bus::new());
        let (commands, receiver) = mpsc::channel(8);
        let supervision = Supervision::new(config, Arc::clone(&bus));
        let task = tokio::spawn(supervise(supervision, receiver));
        Self {
            commands,
            bus,
            task,
        }
    }

    /// Every event from now on, in order.
    #[must_use]
    pub fn events(&self) -> broadcast::Receiver<DaemonEvent> {
        self.bus.subscribe()
    }

    /// The latest event, for a view that renders state rather than history.
    #[must_use]
    pub fn current(&self) -> watch::Receiver<DaemonEvent> {
        self.bus.current()
    }

    /// Starts the daemon again after it halted or the supervisor gave up, with a fresh restart
    /// budget.
    ///
    /// # Errors
    /// [`Error::AlreadyRunning`] when the daemon is up, [`Error::Stopped`] after `stop`.
    pub async fn restart(&self) -> Result<(), Error> {
        let (reply, answer) = oneshot::channel();
        self.commands
            .send(Command::Restart(reply))
            .await
            .map_err(|_| Error::Stopped)?;
        answer.await.map_err(|_| Error::Stopped)?
    }

    /// Stops the daemon gracefully (SIGTERM, then SIGKILL after the grace period) and ends
    /// supervision.
    pub async fn stop(self) {
        let (done, finished) = oneshot::channel();
        if self.commands.send(Command::Stop(done)).await.is_ok() {
            finished.await.ok();
        }
        self.task.await.ok();
    }
}

#[cfg(test)]
#[path = "supervisor_tests.rs"]
mod tests;
