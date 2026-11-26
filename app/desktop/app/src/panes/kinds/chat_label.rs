pub(crate) const NEW_CHAT: &str = "New chat";
pub(crate) const LOADING_CHAT: &str = "Loading…";
pub(crate) const NEW_THREAD: &str = "New thread";

// SPEC: docs/plans/2026-11-24-draft-thread.md#steps (step 2: a draft's tab reads "New thread")
pub(crate) fn unloaded_label(thread: Option<&str>) -> &'static str {
    if thread.is_some_and(store::DraftThread::is_draft) {
        return NEW_THREAD;
    }
    LOADING_CHAT
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_draft_waits_as_a_new_thread_and_a_chat_as_loading() {
        let cases = [
            (Some("draft-u1"), NEW_THREAD),
            (Some("c1"), LOADING_CHAT),
            (None, LOADING_CHAT),
        ];
        for (thread, label) in cases {
            assert_eq!(unloaded_label(thread), label, "{thread:?}");
        }
    }
}
