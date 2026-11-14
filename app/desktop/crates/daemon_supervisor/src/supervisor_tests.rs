use std::path::PathBuf;
use std::time::Duration;

use super::*;
use crate::daemon_paths::DaemonPaths;

fn missing_binary(dir: &std::path::Path) -> SupervisorConfig {
    let paths = DaemonPaths {
        socket: dir.join("d.sock"),
        log: dir.join("d.log"),
    };
    let mut config = SupervisorConfig::new(
        PathBuf::from("/definitely/not/crowbar-api"),
        paths,
        None,
        "test",
    );
    config.timings.respawn_base = Duration::from_millis(1);
    config
}

#[tokio::test]
async fn a_restart_while_the_daemon_is_not_up_is_accepted_and_one_after_stop_is_refused() {
    let dir = tempfile::tempdir().unwrap();
    let supervisor = Supervisor::start(missing_binary(dir.path()));
    let mut events = supervisor.events();
    while !matches!(events.recv().await, Ok(DaemonEvent::Recovering { .. })) {}
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
    let dir = tempfile::tempdir().unwrap();
    let supervisor = Supervisor::start(missing_binary(dir.path()));
    let mut events = supervisor.events();
    let commands = supervisor.commands.clone();
    drop(supervisor);
    let mut last = events.recv().await;
    while matches!(&last, Ok(event) if *event != DaemonEvent::Stopped) {
        last = events.recv().await;
    }
    assert_eq!(
        last,
        Ok(DaemonEvent::Stopped),
        "the stream ended without a Stopped event"
    );
    commands.closed().await;
}
