use std::sync::Arc;
use std::time::Duration;

use browser::AgentAnswer;
use tokio::sync::oneshot;

use crate::wait::pending_forget::PendingForget;

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
    use super::*;
    use crate::wait::Replies;

    #[tokio::test(start_paused = true)]
    async fn a_wait_replaced_by_a_newer_request_with_its_id_fails_at_once() {
        let replies = Arc::new(Replies::default());
        let old = replies.register("1", None);
        let _new = replies.register("1", None);
        let outcome = old.wait(Duration::from_secs(1)).await;
        let dropped = matches!(&outcome, AgentAnswer::Failed(text) if text.contains("dropped"));
        assert!(dropped, "{outcome:?}");
    }
}
