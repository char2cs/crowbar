mod daemon_event;
pub(crate) mod monitoring;
mod readiness;
mod recovery;
mod served_now;
mod spawn;
mod supervise;
pub(crate) mod supervision;
mod supervisor_event;

pub use daemon_event::DaemonEvent;

use std::sync::Arc;

use tokio::sync::{broadcast, mpsc, oneshot, watch};
use tokio::task::JoinHandle;

use crate::config::Config;
use crate::error::Error;
use crate::supervisor::supervise::supervise;
use crate::supervisor::supervision::Bus;
use crate::supervisor::supervision::Command;
use crate::supervisor::supervision::Supervision;
use crate::supervisor::supervisor_event::SupervisorEvent;

const COMMAND_QUEUE: usize = 8;

/// Handle to the running supervision task.
///
/// The shell must `await` [`Supervisor::stop`] before it exits: that is the only way the daemon
/// gets SIGTERM and time to flush its state. Dropping the handle without it asks the task to stop
/// the same way and returns at once, and when the runtime is already shutting down the daemon
/// child is killed outright (it is spawned with `kill_on_drop`).
#[derive(Debug)]
pub struct Supervisor {
    commands: mpsc::Sender<Command>,
    bus: Arc<Bus>,
    task: Option<JoinHandle<()>>,
}

fn stopped(why: impl std::fmt::Display) -> Error {
    tracing::debug!("the supervisor is no longer running: {why}");
    Error::Stopped
}

impl Supervisor {
    /// Starts the daemon and supervises it until [`Supervisor::stop`].
    #[must_use]
    pub fn start(config: Config) -> Self {
        let bus = Arc::new(Bus::default());
        let (commands, receiver) = mpsc::channel(COMMAND_QUEUE);
        let supervision = Supervision::new(config, Arc::clone(&bus));
        let task = tokio::spawn(supervise(supervision, receiver));
        Self {
            commands,
            bus,
            task: Some(task),
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
            .map_err(stopped)?;
        answer.await.map_err(stopped)?
    }

    /// Tells the supervisor this computer's daemon answered someone, such as the shell's stream,
    /// so a launch that was slow to serve is checked now rather than at the next probe.
    pub fn heard_answer(&self) {
        if self.commands.try_send(Command::Answered).is_err() {
            tracing::debug!("the supervisor is busy or stopped; the answer goes unreported");
        }
    }

    /// Stops the daemon gracefully (SIGTERM, then SIGKILL after the grace period) and ends
    /// supervision.
    pub async fn stop(mut self) {
        self.ask_to_stop().await;
        self.join_task().await;
    }

    async fn ask_to_stop(&self) {
        let (done, finished) = oneshot::channel();
        if self.commands.send(Command::Stop(done)).await.is_ok() && finished.await.is_err() {
            crate::journal_sink::emit(&SupervisorEvent::StopUnconfirmed);
        }
    }

    async fn join_task(&mut self) {
        if let Some(Err(error)) = self.finished().await {
            crate::journal_sink::emit(&SupervisorEvent::TaskFailed { error: &error });
        }
    }

    async fn finished(&mut self) -> Option<Result<(), tokio::task::JoinError>> {
        match self.task.take() {
            Some(task) => Some(task.await),
            None => None,
        }
    }
}

impl Drop for Supervisor {
    fn drop(&mut self) {
        let Some(task) = self.task.take() else {
            return;
        };
        let (done, _unwanted) = oneshot::channel();
        if self.commands.try_send(Command::Stop(done)).is_err() {
            task.abort();
        }
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;
    use std::time::Duration;

    use super::*;
    use crate::config::DaemonPaths;

    const NOWHERE: &str = "/crowbar-test-missing";

    fn missing_binary() -> Config {
        let paths = DaemonPaths {
            socket: PathBuf::from(NOWHERE).join("d.sock"),
            log: PathBuf::from(NOWHERE).join("d.log"),
        };
        let mut config = Config::new(
            PathBuf::from("/definitely/not/crowbar-api"),
            paths,
            None,
            "test",
        );
        config.timings.respawn_base = Duration::from_millis(1);
        config
    }

    async fn until_recovering(events: &mut broadcast::Receiver<DaemonEvent>) {
        while !matches!(events.recv().await, Ok(DaemonEvent::Recovering { .. })) {}
    }

    async fn until_stopped(
        events: &mut broadcast::Receiver<DaemonEvent>
    ) -> Result<DaemonEvent, broadcast::error::RecvError> {
        let mut last = events.recv().await;
        while matches!(&last, Ok(event) if *event != DaemonEvent::Stopped) {
            last = events.recv().await;
        }
        last
    }

    #[tokio::test]
    async fn a_restart_while_the_daemon_is_not_up_is_accepted_and_one_after_stop_is_refused() {
        let supervisor = Supervisor::start(missing_binary());
        let mut events = supervisor.events();
        until_recovering(&mut events).await;
        supervisor.restart().await.unwrap();
        let commands = supervisor.commands.clone();
        supervisor.stop().await;
        let (reply, _answer) = oneshot::channel();
        assert!(
            commands.send(Command::Restart(reply)).await.is_err(),
            "supervision has ended"
        );
    }

    #[tokio::test]
    async fn dropping_the_handle_asks_the_task_to_stop_instead_of_leaving_it_running() {
        let supervisor = Supervisor::start(missing_binary());
        let mut events = supervisor.events();
        let commands = supervisor.commands.clone();
        drop(supervisor);
        let last = until_stopped(&mut events).await;
        assert_eq!(
            last,
            Ok(DaemonEvent::Stopped),
            "the stream ended without a Stopped event"
        );
        commands.closed().await;
    }
}
