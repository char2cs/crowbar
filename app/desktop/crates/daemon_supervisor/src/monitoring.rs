use std::process::ExitStatus;

use rustix::process::Signal;
use tokio::sync::mpsc;

use crate::command::Command;
use crate::daemon_event::DaemonEvent;
use crate::daemon_get::get;
use crate::error::Error;
use crate::phase::Phase;
use crate::readiness::describe_exit;
use crate::reply::reply;
use crate::running::Running;
use crate::socket_dial::SocketDial;
use crate::stop::{send, stop_with};
use crate::supervision::Supervision;

type Commands = mpsc::Receiver<Command>;
type Raced = Result<(), Option<Command>>;

const DUMP_LIMIT: usize = 16 * 1024 * 1024;

pub(crate) async fn stop_child(
    supervision: &Supervision,
    child: &mut tokio::process::Child,
) {
    let status = stop_with(
        child,
        Signal::TERM,
        supervision.config.timings.sigterm_grace,
    )
    .await;
    supervision.note("===", &describe_exit(&status));
}

pub(crate) async fn monitor(
    supervision: &mut Supervision,
    mut running: Running,
    commands: &mut Commands,
) -> Phase {
    let mut settled = None;
    while settled.is_none() {
        settled = watch_once(supervision, &mut running, commands).await;
    }
    settled.unwrap_or(Phase::Stop)
}

async fn watch_once(
    supervision: &mut Supervision,
    running: &mut Running,
    commands: &mut Commands,
) -> Option<Phase> {
    tokio::select! {
        status = running.child.wait() => Some(exited(supervision, &status)),
        command = commands.recv() => on_command(supervision, running, command).await,
        Some(()) = running.wedged.recv() => on_wedged(supervision, running, commands).await,
    }
}

fn exited(
    supervision: &Supervision,
    status: &std::io::Result<ExitStatus>,
) -> Phase {
    let reason = describe_exit(status);
    supervision.note("===", &reason);
    Phase::Recover { reason }
}

async fn on_command(
    supervision: &Supervision,
    running: &mut Running,
    command: Option<Command>,
) -> Option<Phase> {
    if let Some(Command::Restart(asked)) = command {
        reply(asked, Err(Error::AlreadyRunning));
        return None;
    }
    Some(stop_running(supervision, running, command).await)
}

async fn on_wedged(
    supervision: &Supervision,
    running: &mut Running,
    commands: &mut Commands,
) -> Option<Phase> {
    match kill_unless_stopped(supervision, running, commands).await {
        Ok(()) => None,
        Err(stop) => Some(stop_running(supervision, running, stop).await),
    }
}

async fn stop_running(
    supervision: &Supervision,
    running: &mut Running,
    command: Option<Command>,
) -> Phase {
    stop_child(supervision, &mut running.child).await;
    supervision.bus.emit(DaemonEvent::Stopped);
    if let Some(Command::Stop(done)) = command {
        reply(done, ());
    }
    Phase::Stop
}

async fn kill_unless_stopped(
    supervision: &Supervision,
    running: &mut Running,
    commands: &mut Commands,
) -> Raced {
    let kill = std::pin::pin!(kill_wedged(supervision, &mut running.child));
    race_stop(kill, commands).await
}

async fn race_stop(
    mut kill: std::pin::Pin<&mut impl Future<Output = ()>>,
    commands: &mut Commands,
) -> Raced {
    let mut settled = None;
    while settled.is_none() {
        settled = race_once(&mut kill, commands).await;
    }
    settled.unwrap_or(Ok(()))
}

async fn race_once(
    kill: &mut std::pin::Pin<&mut impl Future<Output = ()>>,
    commands: &mut Commands,
) -> Option<Raced> {
    tokio::select! {
        () = &mut *kill => Some(Ok(())),
        command = commands.recv() => refuse_restart(command),
    }
}

fn refuse_restart(command: Option<Command>) -> Option<Raced> {
    if let Some(Command::Restart(asked)) = command {
        reply(asked, Err(Error::AlreadyRunning));
        return None;
    }
    Some(Err(command))
}

async fn kill_wedged(
    supervision: &Supervision,
    child: &mut tokio::process::Child,
) {
    let t = &supervision.config.timings;
    let dump = tokio::time::timeout(
        t.dump_timeout,
        get(
            &SocketDial::new(&supervision.config.paths.socket),
            "/debug/pprof/goroutine?debug=2",
            DUMP_LIMIT,
        ),
    )
    .await;
    match dump {
        Ok(Ok(body)) => supervision.note(
            "=== watchdog goroutine dump",
            &String::from_utf8_lossy(&body),
        ),
        Ok(Err(e)) => tracing::debug!("no goroutine dump: {e}"),
        Err(elapsed) => tracing::debug!("no goroutine dump within {:?}: {elapsed}", t.dump_timeout),
    }
    send(child, Signal::QUIT);
    if tokio::time::timeout(t.sigquit_grace, child.wait())
        .await
        .is_err()
        && let Err(e) = child.kill().await
    {
        tracing::warn!("cannot kill the wedged daemon: {e}");
    }
}

#[cfg(test)]
mod tests {
    use tokio::sync::oneshot;

    use super::*;

    #[test]
    fn a_stop_ends_the_race_and_a_closed_queue_is_a_stop_too() {
        let (done, _heard) = oneshot::channel();
        assert!(matches!(
            refuse_restart(Some(Command::Stop(done))),
            Some(Err(Some(Command::Stop(_))))
        ));
        assert!(matches!(refuse_restart(None), Some(Err(None))));
    }

    #[tokio::test]
    async fn a_restart_during_the_race_is_refused_and_the_race_goes_on() {
        let (asked, answer) = oneshot::channel();
        assert!(refuse_restart(Some(Command::Restart(asked))).is_none());
        assert!(matches!(answer.await.unwrap(), Err(Error::AlreadyRunning)));
    }
}
