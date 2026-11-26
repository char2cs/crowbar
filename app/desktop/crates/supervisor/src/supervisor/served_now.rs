mod served_now_event;

use crate::health::{Health, SocketDial, check_health};
use crate::supervisor::served_now::served_now_event::ServedNowEvent;
use crate::supervisor::supervision::Supervision;

pub(crate) async fn served_now(supervision: &Supervision) -> Option<Option<u32>> {
    let dial = SocketDial::new(&supervision.config.paths.socket);
    let budget = supervision.config.timings.health_probe_timeout;
    match tokio::time::timeout(budget, check_health(&dial)).await {
        Ok(Ok(Health::Ready { pid })) => Some(pid),
        Ok(Ok(_)) => None,
        Ok(Err(error)) => {
            crate::journal_sink::emit(&ServedNowEvent::NotServing { error: &error });
            None
        },
        Err(elapsed) => {
            crate::journal_sink::emit(&ServedNowEvent::NotAnswering { budget, elapsed });
            None
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::supervision;

    #[tokio::test]
    async fn a_daemon_that_cannot_be_reached_does_not_serve() {
        assert_eq!(served_now(&supervision()).await, None);
    }
}
