use std::collections::BTreeMap;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use browser_nav::AgentAnswer;
use serde_json::Value;
use tokio::sync::oneshot;

use crate::expectation::Expectation;
use crate::pending_forget::PendingForget;
use crate::pending_reply::PendingReply;
use crate::reply_scope::{ReplyScope, merged};

/// The requests that wait for an answer, shared by the loop that serves the daemon and the shell
/// that does the work.
///
/// Each wait names who may end it: the shell always, a page only for its own pane, so a page
/// cannot answer for another tab. A navigation waits for the load that follows it, never for
/// one that was already running.
#[derive(Debug, Default)]
pub struct Replies {
    pending: Mutex<BTreeMap<String, PendingReply>>,
}

impl Replies {
    /// Starts waiting for request `id`. `pane` is the tab the request is about, when it has one;
    /// the page of that pane may answer too.
    #[must_use]
    pub fn register(
        self: &Arc<Self>,
        id: &str,
        pane: Option<&str>,
    ) -> Expectation {
        let (sender, receiver) = oneshot::channel();
        let scope = pane.map_or(ReplyScope::Shell, |pane| ReplyScope::Page(pane.to_owned()));
        self.lock()
            .insert(id.to_owned(), PendingReply { scope, sender });
        Expectation::new(
            Arc::clone(self) as Arc<dyn PendingForget>,
            id.to_owned(),
            receiver,
        )
    }

    /// Whether request `id` is still waiting for an answer.
    pub fn is_waiting(
        &self,
        id: &str,
    ) -> bool {
        self.lock().contains_key(id)
    }

    /// The shell answers request `id`. `false` when nobody waits for it any more.
    pub fn complete(
        &self,
        id: &str,
        answer: AgentAnswer,
    ) -> bool {
        let Some(pending) = self.lock().remove(id) else {
            return false;
        };
        pending.sender.send(answer).is_ok()
    }

    /// The page of `pane` answers request `id`. Refused, with the wait left running, when the
    /// request is about another pane or is not waiting for a page.
    pub fn from_page(
        &self,
        pane: &str,
        id: &str,
        answer: AgentAnswer,
    ) -> bool {
        let mut pending = self.lock();
        let Some(entry) = pending.get(id) else {
            return false;
        };
        if !matches!(&entry.scope, ReplyScope::Page(owner) if owner == pane) {
            return false;
        }
        let Some(entry) = pending.remove(id) else {
            return false;
        };
        drop(pending);
        entry.sender.send(answer).is_ok()
    }

    /// Request `id` is answered with `result` once `pane` has navigated and finished loading.
    pub fn after_load(
        &self,
        id: &str,
        pane: &str,
        result: Value,
    ) -> bool {
        let mut pending = self.lock();
        let Some(entry) = pending.get_mut(id) else {
            return false;
        };
        entry.scope = ReplyScope::LoadEnd {
            pane: pane.to_owned(),
            armed: false,
            result,
        };
        true
    }

    /// `pane` started loading a page: the requests waiting for its next load end now wait for
    /// this one.
    pub fn loading(
        &self,
        pane: &str,
    ) {
        for entry in self.lock().values_mut() {
            arm(&mut entry.scope, pane);
        }
    }

    /// `pane` finished loading and shows `page`: answers the requests that waited for this
    /// load with their result and, when both are objects, the fields of `page` added to it.
    pub fn loaded(
        &self,
        pane: &str,
        page: &Value,
    ) {
        let ready: Vec<String> = self
            .lock()
            .iter()
            .filter(|(_, entry)| is_armed_for(&entry.scope, pane))
            .map(|(id, _)| id.clone())
            .collect();
        for id in ready {
            self.finish_load(&id, page);
        }
    }

    fn finish_load(
        &self,
        id: &str,
        page: &Value,
    ) {
        let Some(entry) = self.lock().remove(id) else {
            return;
        };
        if let ReplyScope::LoadEnd { result, .. } = entry.scope {
            let result = merged(result, page);
            let delivered = entry.sender.send(AgentAnswer::Done(result)).is_ok();
            tracing::debug!(delivered, "a navigation finished loading");
        }
    }

    fn lock(&self) -> MutexGuard<'_, BTreeMap<String, PendingReply>> {
        self.pending.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

impl PendingForget for Replies {
    fn forget(
        &self,
        id: &str,
    ) {
        self.lock().remove(id);
    }
}

fn arm(
    scope: &mut ReplyScope,
    pane: &str,
) {
    match scope {
        ReplyScope::LoadEnd {
            pane: waiting,
            armed,
            ..
        } if waiting == pane => *armed = true,
        _ => {},
    }
}

fn is_armed_for(
    scope: &ReplyScope,
    pane: &str,
) -> bool {
    matches!(scope, ReplyScope::LoadEnd { pane: waiting, armed: true, .. } if waiting == pane)
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use serde_json::json;

    use super::*;

    const LIMIT: Duration = Duration::from_secs(1);

    fn done(value: &str) -> AgentAnswer {
        AgentAnswer::Done(json!(value))
    }

    #[tokio::test(start_paused = true)]
    async fn the_shell_answers_any_waiting_request_once() {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", None);
        assert!(replies.complete("1", done("done")));
        assert!(!replies.complete("1", done("again")), "answered once");
        assert_eq!(wait.wait(LIMIT).await, done("done"));
    }

    #[tokio::test(start_paused = true)]
    async fn a_page_answers_only_for_its_own_pane() {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", Some("b1"));
        assert!(
            !replies.from_page("b2", "1", done("forged")),
            "another pane"
        );
        assert!(replies.from_page("b1", "1", done("mine")));
        assert_eq!(wait.wait(LIMIT).await, done("mine"));
    }

    #[tokio::test(start_paused = true)]
    async fn a_request_that_is_not_about_a_pane_takes_no_page_answer() {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", None);
        assert!(!replies.from_page("b1", "1", done("forged")));
        assert!(replies.complete("1", done("shell")));
        assert_eq!(wait.wait(LIMIT).await, done("shell"));
    }

    #[tokio::test(start_paused = true)]
    async fn a_navigation_is_answered_by_the_load_it_started_not_by_one_already_running() {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", Some("b1"));
        let page = json!({"url":"http://a.test/","loading":false});
        assert!(replies.after_load("1", "b1", json!({"tab":"b1"})));
        replies.loaded("b1", &page);
        replies.loading("b2");
        replies.loaded("b2", &page);
        assert!(
            replies.is_waiting("1"),
            "neither load was the one it waited for"
        );
        replies.loading("b1");
        replies.loaded("b1", &page);
        let expected = json!({"tab":"b1","url":"http://a.test/","loading":false});
        assert_eq!(wait.wait(LIMIT).await, AgentAnswer::Done(expected));
    }

    #[tokio::test(start_paused = true)]
    async fn a_wait_that_nobody_answers_ends_at_its_limit_and_is_forgotten() {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", Some("b1"));
        let outcome = wait.wait(LIMIT).await;
        assert!(
            matches!(&outcome, AgentAnswer::Failed(text) if text.contains("no answer")),
            "{outcome:?}"
        );
        assert!(!replies.complete("1", done("late")), "forgotten");
    }

    #[tokio::test(start_paused = true)]
    async fn a_wait_that_is_dropped_is_forgotten() {
        let replies = Arc::new(Replies::default());
        drop(replies.register("1", None));
        assert!(!replies.complete("1", done("late")));
    }
}
