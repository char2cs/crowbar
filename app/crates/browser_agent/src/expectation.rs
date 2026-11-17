use std::sync::Arc;
use std::time::Duration;

use browser_nav::AgentAnswer;
use tokio::sync::oneshot;

use crate::pending_forget::PendingForget;

/// A request's wait for its answer. Dropping it, or giving up, forgets the request.
pub struct Expectation {
    forget: Arc<dyn PendingForget>,
    entry: (String, u64),
    receiver: oneshot::Receiver<AgentAnswer>,
}

impl std::fmt::Debug for Expectation {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("Expectation")
            .field("id", &self.entry.0)
            .finish_non_exhaustive()
    }
}

impl Expectation {
    pub(crate) fn new(
        forget: Arc<dyn PendingForget>,
        entry: (String, u64),
        receiver: oneshot::Receiver<AgentAnswer>,
    ) -> Self {
        Self {
            forget,
            entry,
            receiver,
        }
    }

    /// The answer, or a failure once `limit` has passed with none.
    pub async fn wait(
        mut self,
        limit: Duration,
    ) -> AgentAnswer {
        let answer = tokio::time::timeout(limit, &mut self.receiver).await;
        match answer {
            Ok(Ok(answer)) => answer,
            Ok(Err(dropped)) => {
                AgentAnswer::Failed(format!("the shell dropped the request: {dropped}"))
            },
            Err(elapsed) => AgentAnswer::Failed(format!(
                "no answer within {} ms ({elapsed})",
                limit.as_millis()
            )),
        }
    }
}

impl Drop for Expectation {
    fn drop(&mut self) {
        self.forget.forget(&self.entry);
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use super::*;

    #[derive(Default)]
    struct Forgotten(Mutex<Vec<(String, u64)>>);

    impl PendingForget for Forgotten {
        fn forget(
            &self,
            entry: &(String, u64),
        ) {
            self.0.lock().unwrap().push(entry.clone());
        }
    }

    const LIMIT: Duration = Duration::from_secs(1);

    async fn outcome_of(reply: Option<AgentAnswer>) -> (AgentAnswer, Vec<(String, u64)>) {
        let forgotten = Arc::new(Forgotten::default());
        let (sender, receiver) = oneshot::channel();
        let wait = Expectation::new(
            Arc::clone(&forgotten) as Arc<dyn PendingForget>,
            ("5".into(), 7),
            receiver,
        );
        let sent = reply.map(|answer| sender.send(answer));
        assert!(
            sent.is_none_or(|sent| sent.is_ok()),
            "the wait still listens"
        );
        let outcome = wait.wait(LIMIT).await;
        let seen = forgotten.0.lock().unwrap().clone();
        (outcome, seen)
    }

    #[tokio::test(start_paused = true)]
    async fn a_wait_ends_with_the_answer_or_a_failure_and_is_always_forgotten() {
        let (answered, seen) = outcome_of(Some(AgentAnswer::Done("ok".into()))).await;
        assert_eq!(
            (answered, seen),
            (AgentAnswer::Done("ok".into()), vec![("5".to_owned(), 7)])
        );
        let (silent, seen) = outcome_of(None).await;
        assert!(matches!(silent, AgentAnswer::Failed(_)), "{silent:?}");
        assert_eq!(seen, [("5".to_owned(), 7)]);
    }
}
