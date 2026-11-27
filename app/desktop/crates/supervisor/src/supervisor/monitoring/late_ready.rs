use super::running::Running;
use super::stale_daemon::refuse;
use crate::supervisor::daemon_event::DaemonEvent;
use crate::supervisor::readiness::pid_mismatch;
use crate::supervisor::served_now::served_now;
use crate::supervisor::supervision::Phase;
use crate::supervisor::supervision::Supervision;

pub(super) async fn answered(
    supervision: &Supervision,
    running: &mut Running,
) -> Option<Phase> {
    let ready = matches!(
        *supervision.bus.current().borrow(),
        DaemonEvent::Ready { .. }
    );
    if ready {
        return None;
    }
    let pid = served_now(supervision).await?;
    let spawned = running.child.id();
    if let Some(reason) = pid_mismatch(pid, spawned) {
        return Some(refuse(supervision, &mut running.child, reason).await);
    }
    supervision.bus.emit(DaemonEvent::Ready {
        pid: pid.or(spawned),
    });
    None
}

#[cfg(test)]
mod tests {
    use tokio::sync::mpsc;
    use tokio_util::sync::CancellationToken;

    use super::*;
    use crate::supervisor::monitoring::Guard;
    use crate::test_support::supervision;

    #[ignore = "real-infra: spawns a real sleeping child process"]
    fn running() -> Running {
        let child = tokio::process::Command::new("sleep")
            .arg("60")
            .kill_on_drop(true)
            .spawn()
            .unwrap();
        Running {
            _watchdog: Guard::new(CancellationToken::new()),
            child,
            wedged: mpsc::channel(1).1,
            healthy: mpsc::channel(1).1,
        }
    }

    #[ignore = "real-infra: weighs an answer while holding a real child process"]
    async fn weigh(current: DaemonEvent) -> (bool, DaemonEvent, bool) {
        let supervision = supervision();
        supervision.bus.emit(current);
        let mut running = running();
        let phase = answered(&supervision, &mut running).await;
        let now = supervision.bus.current().borrow().clone();
        let alive = running.child.try_wait().unwrap().is_none();
        (phase.is_none(), now, alive)
    }

    #[tokio::test]
    #[ignore = "real-infra: holds a real child process while the answer is weighed"]
    async fn an_answer_changes_nothing_once_ready_or_while_nothing_serves() {
        let ready = DaemonEvent::Ready { pid: Some(7) };
        assert_eq!(weigh(ready.clone()).await, (true, ready, true));
        let starting = DaemonEvent::Starting;
        assert_eq!(weigh(starting.clone()).await, (true, starting, true));
    }
}
