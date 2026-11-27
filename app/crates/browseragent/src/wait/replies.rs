use std::collections::BTreeMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use browser::AgentAnswer;
use serde_json::Value;
use tokio::sync::oneshot;

use crate::wait::arming::Arming;
use crate::wait::delivery::Delivery;
use crate::wait::expectation::Expectation;
use crate::wait::pending_forget::PendingForget;
use crate::wait::pending_reply::PendingReply;
use crate::wait::reply_scope::{ReplyScope, merged};

#[derive(Debug, Default)]
pub struct Replies {
    pending: Mutex<BTreeMap<String, PendingReply>>,
    serial: AtomicU64,
}

impl Replies {
    #[must_use]
    pub fn register(
        self: &Arc<Self>,
        id: &str,
        pane: Option<&str>,
    ) -> Expectation {
        let (sender, receiver) = oneshot::channel();
        let scope = pane.map_or(ReplyScope::Shell, |pane| ReplyScope::Page(pane.to_owned()));
        let serial = self.serial.fetch_add(1, Ordering::Relaxed);
        self.lock().insert(
            id.to_owned(),
            PendingReply {
                serial,
                scope,
                sender,
            },
        );
        Expectation::new(
            Arc::clone(self) as Arc<dyn PendingForget>,
            (id.to_owned(), serial),
            receiver,
        )
    }

    pub fn is_waiting(
        &self,
        id: &str,
    ) -> bool {
        self.lock().contains_key(id)
    }

    pub fn complete(
        &self,
        id: &str,
        answer: AgentAnswer,
    ) -> Delivery {
        let Some(pending) = self.lock().remove(id) else {
            return Delivery::NotWaiting;
        };
        pending.deliver(answer)
    }

    pub fn from_page(
        &self,
        pane: &str,
        id: &str,
        answer: AgentAnswer,
    ) -> Delivery {
        let mut pending = self.lock();
        let Some(entry) = pending.get(id) else {
            return Delivery::NotWaiting;
        };
        if !matches!(&entry.scope, ReplyScope::Page(owner) if owner == pane) {
            return Delivery::Refused;
        }
        let Some(entry) = pending.remove(id) else {
            return Delivery::NotWaiting;
        };
        drop(pending);
        entry.deliver(answer)
    }

    pub fn after_load(
        &self,
        id: &str,
        pane: &str,
        result: Value,
    ) -> Arming {
        let mut pending = self.lock();
        let Some(entry) = pending.get_mut(id) else {
            return Arming::NotWaiting;
        };
        entry.scope = ReplyScope::LoadEnd {
            pane: pane.to_owned(),
            armed: false,
            result,
        };
        Arming::Armed
    }

    pub fn abandon_loads(
        &self,
        is_open: impl Fn(&str) -> bool,
        reason: &str,
    ) {
        let gone: Vec<String> = self
            .lock()
            .iter()
            .filter(|(_, entry)| entry.scope.waits_on_closed(&is_open))
            .map(|(id, _)| id.clone())
            .collect();
        for id in gone {
            self.fail(&id, reason);
        }
    }

    fn fail(
        &self,
        id: &str,
        reason: &str,
    ) {
        let Some(entry) = self.lock().remove(id) else {
            return;
        };
        let delivery = entry.deliver(AgentAnswer::Failed(reason.to_owned()));
        tracing::debug!(?delivery, id, "a navigation lost its pane");
    }

    pub fn loading(
        &self,
        pane: &str,
    ) {
        for entry in self.lock().values_mut() {
            entry.scope.arm(pane);
        }
    }

    pub fn loaded(
        &self,
        pane: &str,
        page: &Value,
    ) {
        let ready: Vec<String> = self
            .lock()
            .iter()
            .filter(|(_, entry)| entry.scope.is_armed_for(pane))
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
        (id, serial): &(String, u64),
    ) {
        let mut pending = self.lock();
        if pending.get(id).is_some_and(|entry| entry.serial == *serial) {
            pending.remove(id);
        }
    }
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

    struct Case {
        scope: Option<&'static str>,
        page: Option<&'static str>,
        expected: Delivery,
        again: Delivery,
    }

    const CASES: &[Case] = &[
        Case {
            scope: None,
            page: None,
            expected: Delivery::Delivered,
            again: Delivery::NotWaiting,
        },
        Case {
            scope: Some("b1"),
            page: Some("b1"),
            expected: Delivery::Delivered,
            again: Delivery::NotWaiting,
        },
        Case {
            scope: Some("b1"),
            page: Some("b2"),
            expected: Delivery::Refused,
            again: Delivery::Delivered,
        },
        Case {
            scope: None,
            page: Some("b1"),
            expected: Delivery::Refused,
            again: Delivery::Delivered,
        },
    ];

    fn offer(case: &Case) {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", case.scope);
        let offered = match case.page {
            Some(pane) => replies.from_page(pane, "1", done("page")),
            None => replies.complete("1", done("shell")),
        };
        assert_eq!(offered, case.expected, "{:?} {:?}", case.scope, case.page);
        let delivered = case.expected == Delivery::Delivered;
        assert_eq!(
            replies.is_waiting("1"),
            !delivered,
            "a refusal keeps the wait"
        );
        let again = replies.complete("1", done("again"));
        assert_eq!(again, case.again, "answered once");
        drop(wait);
    }

    fn every_case() {
        for case in CASES {
            offer(case);
        }
    }

    #[tokio::test(start_paused = true)]
    async fn the_shell_answers_any_request_once_and_a_page_only_its_own() {
        every_case();
    }

    #[tokio::test(start_paused = true)]
    async fn a_navigation_is_answered_by_the_load_it_started_not_by_one_already_running() {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", Some("b1"));
        let page = json!({"url":"http://a.test/","loading":false});
        let armed = replies.after_load("1", "b1", json!({"tab":"b1"}));
        assert_eq!(armed, Arming::Armed);
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
        let late = replies.after_load("1", "b1", json!({}));
        assert_eq!(late, Arming::NotWaiting);
    }

    #[tokio::test(start_paused = true)]
    async fn a_wait_that_nobody_answers_or_that_is_dropped_is_forgotten() {
        let replies = Arc::new(Replies::default());
        let wait = replies.register("1", Some("b1"));
        let outcome = wait.wait(LIMIT).await;
        assert!(
            matches!(&outcome, AgentAnswer::Failed(text) if text.contains("no answer")),
            "{outcome:?}"
        );
        assert_eq!(replies.complete("1", done("late")), Delivery::NotWaiting);
        drop(replies.register("2", None));
        assert_eq!(replies.complete("2", done("late")), Delivery::NotWaiting);
    }

    #[tokio::test(start_paused = true)]
    async fn a_navigation_whose_pane_is_gone_fails_at_once_and_the_others_keep_waiting() {
        let replies = Arc::new(Replies::default());
        let lost = replies.register("1", Some("b1"));
        let kept = replies.register("2", Some("b2"));
        replies.after_load("1", "b1", json!({}));
        replies.after_load("2", "b2", json!({}));
        replies.abandon_loads(|pane| pane == "b2", "the tab was closed");
        assert_eq!(
            lost.wait(LIMIT).await,
            AgentAnswer::Failed("the tab was closed".into())
        );
        assert!(replies.is_waiting("2"));
        drop(kept);
    }

    #[tokio::test(start_paused = true)]
    async fn an_old_wait_that_ends_does_not_forget_a_newer_request_with_the_same_id() {
        let replies = Arc::new(Replies::default());
        let old = replies.register("1", None);
        let new = replies.register("1", None);
        drop(old);
        assert!(replies.is_waiting("1"), "the newer request still waits");
        assert_eq!(replies.complete("1", done("mine")), Delivery::Delivered);
        assert_eq!(new.wait(LIMIT).await, done("mine"));
    }
}
