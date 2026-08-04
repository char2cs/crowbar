//! The branch-review thread/state model and its mutators.
//!
//! Ported from
//! `web/src/features/workspace/stores/slices/branch-review-slice.ts`
//! (156 lines): the four types (`ReviewMessage`, `ReviewThread`,
//! `ReviewConversation`, and the state shape itself) plus every mutator the
//! TS file's `createBranchReviewSlice` factory exposes, **as one unit** —
//! `native/mapping/tier-a-denominator.md` §7 finds nothing in this file
//! prunable at export granularity.
//!
//! # Shape: 12 closure-bound mutators become 11 free functions, one dropped
//!
//! In TS, `createBranchReviewSlice` is a Zustand `StateCreator`: a factory
//! closure that returns an object of bound mutator methods, each wrapping a
//! trivial, pure `BranchReviewState -> BranchReviewState` (or narrower,
//! `ReviewThread[] -> ReviewThread[]`) transition in an Immer `set(...)`
//! call. The survey's own prose names these "12 pure mutators", but they are
//! not separately exported TS symbols today — they are properties of one
//! factory's return value, so §7's export-liveness table (correctly) audits
//! `createBranchReviewSlice` as a single LIVE unit and does not, because it
//! cannot, verify the 12 individually.
//!
//! `crowbar-core` has no reactive shell (no Zustand, no `set`, no Immer) —
//! that belongs to whatever Phase-4/`crowbar-state` code eventually owns a
//! `BranchReviewState` behind a real store. So this port does not reproduce
//! the `StateCreator` closure shape at all (there is nothing in
//! `crowbar-core` for it to close over); each mutator becomes a plain
//! `fn(&mut BranchReviewState, ..) -> ()` free function instead, taking
//! exactly the sub-state each TS `set((s) => { s.branchReview.X = ... })`
//! callback touched. This is a deliberate refactor, not a mechanical
//! translation — the alternative (a Rust trait or struct-of-closures
//! mirroring `BranchReviewSlice`) would just re-invent the reactive shell
//! this crate is not supposed to own.
//!
//! **One of the 12 is not ported: `resolveReviewThread`.** Its own TS doc
//! comment already marks it `@deprecated` ("Kept for backward compat... Use
//! setReviewThreadResolved instead"), and its body (`find` the thread by id,
//! `if (t) t.isResolved = true`) is identical to
//! [`set_review_thread_resolved`]'s for a hard-coded `isResolved: true` —
//! confirmed by reading both side by side, not just trusting the doc
//! comment's own claim. `git grep -n resolveReviewThread web/src` (outside the
//! slice's own definition and its one dedicated legacy-compat test) turns up
//! zero callers anywhere in `web/src` — the deprecation is not aspirational,
//! nothing calls it. Its reason to exist in TS is a JS/store-API concern
//! this port does not inherit: a `Zustand` store's public shape may be
//! called by code this repo cannot see (a stale closure, a devtools
//! extension, a saved macro), so removing a method is a breaking change even
//! at zero known callers. `crowbar-core::review` has no such external
//! surface yet — nothing outside this same workspace can call a Rust
//! function that does not exist yet — so carrying a callable-but-never-called
//! backward-compat shim forward would be manufacturing dead code on day one,
//! against this port's own stated preference for live code paths (see
//! `crate::git`'s module doc on `ParsedHunk`). Not ported; its behaviour is
//! fully available via `set_review_thread_resolved(state, id, true)`.

use crowbar_proto::domain_git::MergeStrategy;

/// One message in a review thread — the root comment, or a reply. Mirrors
/// TS `ReviewMessage`.
#[derive(Debug, Clone, PartialEq)]
pub struct ReviewMessage {
    pub id: String,
    pub author: Option<String>,
    pub is_agent: bool,
    pub body: String,
    pub created_at: String,
}

/// Which half of a diff a thread is anchored to. Mirrors the TS literal
/// union `ReviewThread['side']` (`'old' | 'new'`) — a strict two-value
/// domain type, not the looser wire string it is built from (see
/// `super::api`'s module doc for that boundary).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ThreadSide {
    Old,
    New,
}

/// One review comment thread, anchored to a line range on one side of one
/// file. Mirrors TS `ReviewThread`.
#[derive(Debug, Clone, PartialEq)]
pub struct ReviewThread {
    pub id: String,
    pub file_path: String,
    pub line_number: i64,
    pub start_line: i64,
    pub end_line: i64,
    pub side: ThreadSide,
    pub messages: Vec<ReviewMessage>,
    pub is_resolved: bool,
}

/// One entry in the branch-review conversations list. Mirrors TS
/// `ReviewConversation`.
///
/// See `super::api`'s module doc: the current backend contract never
/// populates this list (a confirmed, not assumed, finding), so every
/// `ReviewConversation` this port can currently construct is authored by a
/// test, not by a real fetch. The type and its mutators are ported anyway —
/// `native/mapping/tier-a-denominator.md` §7 audits `branchReview.conversations`,
/// `setBranchReviewConversations`, and `addReviewConversation` as LIVE store
/// surface, and this item's scope is that surface, not the wire's current
/// payload.
#[derive(Debug, Clone, PartialEq)]
pub struct ReviewConversation {
    pub id: String,
    pub title: String,
    pub age: String,
    pub is_active: bool,
}

/// `diffStatus`'s four states. Mirrors TS `BranchReviewState['diffStatus']`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum DiffStatus {
    #[default]
    Idle,
    Loading,
    Loaded,
    Error,
}

/// The whole branch-review surface's state. Mirrors TS `BranchReviewState`.
#[derive(Debug, Clone, PartialEq)]
pub struct BranchReviewState {
    pub description: String,
    pub merge_strategy: MergeStrategy,
    pub diff_status: DiffStatus,
    pub threads: Vec<ReviewThread>,
    pub conversations: Vec<ReviewConversation>,
    /// The changed file the review surface has been asked to scroll to, by
    /// PATH. A path, not an index or a composite key: the surface is fed by
    /// the files summary and addresses everything by path (mirrors the TS
    /// field's own doc comment, which records that an index-based key this
    /// replaced resolved against a whole-diff cache that no longer exists,
    /// so every click silently resolved to nothing).
    pub reveal_file_path: Option<String>,
    /// Bumped on every [`reveal_file`] call so requesting the SAME file
    /// twice reveals it again — the path alone would compare equal and a
    /// change-detecting effect would not re-run.
    pub reveal_file_nonce: u64,
}

impl Default for BranchReviewState {
    /// Mirrors `INITIAL_BRANCH_REVIEW_STATE`.
    fn default() -> Self {
        Self {
            description: String::new(),
            merge_strategy: MergeStrategy::Merge,
            diff_status: DiffStatus::default(),
            threads: Vec::new(),
            conversations: Vec::new(),
            reveal_file_path: None,
            reveal_file_nonce: 0,
        }
    }
}

/// Mirrors `setBranchReviewDescription`.
pub fn set_branch_review_description(state: &mut BranchReviewState, description: String) {
    state.description = description;
}

/// Mirrors `setBranchReviewMergeStrategy`.
pub fn set_branch_review_merge_strategy(state: &mut BranchReviewState, strategy: MergeStrategy) {
    state.merge_strategy = strategy;
}

/// Mirrors `setBranchReviewDiffStatus`.
pub fn set_branch_review_diff_status(state: &mut BranchReviewState, status: DiffStatus) {
    state.diff_status = status;
}

/// Ask the review surface to scroll to a changed file. Mirrors
/// `revealBranchReviewFile`. Always bumps [`BranchReviewState::reveal_file_nonce`],
/// even for a repeat request of the same path — see that field's doc comment
/// for why.
pub fn reveal_file(state: &mut BranchReviewState, path: String) {
    state.reveal_file_path = Some(path);
    state.reveal_file_nonce += 1;
}

/// Mirrors `addReviewThread`.
pub fn add_review_thread(state: &mut BranchReviewState, thread: ReviewThread) {
    state.threads.push(thread);
}

/// Mirrors `removeReviewThread`.
pub fn remove_review_thread(state: &mut BranchReviewState, thread_id: &str) {
    state.threads.retain(|t| t.id != thread_id);
}

/// Mirrors `addReviewMessage`. A no-op if `thread_id` matches no thread,
/// same as the TS `if (t) t.messages.push(message)` guard.
pub fn add_review_message(state: &mut BranchReviewState, thread_id: &str, message: ReviewMessage) {
    if let Some(t) = state.threads.iter_mut().find(|t| t.id == thread_id) {
        t.messages.push(message);
    }
}

/// Two-way: pass `is_resolved: false` to reopen. A no-op for an unknown id.
/// Mirrors `setReviewThreadResolved`, and (for `is_resolved: true`) also the
/// now-dropped `resolveReviewThread` — see the module doc.
pub fn set_review_thread_resolved(state: &mut BranchReviewState, thread_id: &str, is_resolved: bool) {
    if let Some(t) = state.threads.iter_mut().find(|t| t.id == thread_id) {
        t.is_resolved = is_resolved;
    }
}

/// Insert if `thread.id` is new; replace in place (by id) if it already
/// exists. Mirrors `upsertReviewThread`.
pub fn upsert_review_thread(state: &mut BranchReviewState, thread: ReviewThread) {
    match state.threads.iter().position(|t| t.id == thread.id) {
        Some(idx) => state.threads[idx] = thread,
        None => state.threads.push(thread),
    }
}

/// Mirrors `setBranchReviewConversations`.
pub fn set_branch_review_conversations(
    state: &mut BranchReviewState,
    conversations: Vec<ReviewConversation>,
) {
    state.conversations = conversations;
}

/// Mirrors `addReviewConversation`, which `unshift`s (prepends) rather than
/// appends.
pub fn add_review_conversation(state: &mut BranchReviewState, conversation: ReviewConversation) {
    state.conversations.insert(0, conversation);
}

#[cfg(test)]
mod tests {
    use super::{
        BranchReviewState, DiffStatus, ReviewConversation, ReviewMessage, ReviewThread,
        ThreadSide, add_review_conversation, add_review_message, add_review_thread, reveal_file,
        remove_review_thread, set_branch_review_conversations, set_branch_review_description,
        set_branch_review_diff_status, set_branch_review_merge_strategy,
        set_review_thread_resolved, upsert_review_thread,
    };
    use crowbar_proto::domain_git::MergeStrategy;

    fn thread(id: &str) -> ReviewThread {
        ReviewThread {
            id: id.to_string(),
            file_path: "src/foo.ts".to_string(),
            line_number: 10,
            start_line: 8,
            end_line: 12,
            side: ThreadSide::New,
            messages: Vec::new(),
            is_resolved: false,
        }
    }

    fn message(id: &str, body: &str) -> ReviewMessage {
        ReviewMessage {
            id: id.to_string(),
            author: Some("char2cs".to_string()),
            is_agent: false,
            body: body.to_string(),
            created_at: "2026-01-01T00:00:00Z".to_string(),
        }
    }

    // --- ported from web/src/__tests__/features/workspace/stores/branch-review-slice.test.ts ---

    #[test]
    fn starts_with_no_reveal_request() {
        let state = BranchReviewState::default();
        assert_eq!(state.reveal_file_path, None);
        assert_eq!(state.reveal_file_nonce, 0);
    }

    #[test]
    fn reveal_file_records_the_path_and_bumps_the_nonce() {
        let mut state = BranchReviewState::default();
        reveal_file(&mut state, "src/foo.ts".to_string());
        assert_eq!(state.reveal_file_path.as_deref(), Some("src/foo.ts"));
        assert_eq!(state.reveal_file_nonce, 1);
    }

    #[test]
    fn reveal_file_bumps_the_nonce_again_for_the_same_path() {
        let mut state = BranchReviewState::default();
        reveal_file(&mut state, "src/foo.ts".to_string());
        reveal_file(&mut state, "src/foo.ts".to_string());
        assert_eq!(state.reveal_file_path.as_deref(), Some("src/foo.ts"));
        assert_eq!(state.reveal_file_nonce, 2);
    }

    #[test]
    fn upsert_review_thread_inserts_a_new_thread_when_id_is_absent() {
        let mut state = BranchReviewState::default();
        upsert_review_thread(&mut state, thread("t-a"));
        assert_eq!(state.threads.len(), 1);
        assert_eq!(state.threads[0].id, "t-a");
        assert_eq!(state.threads[0].start_line, 8);
        assert_eq!(state.threads[0].end_line, 12);
        assert_eq!(state.threads[0].side, ThreadSide::New);
    }

    #[test]
    fn upsert_review_thread_merges_replaces_by_id_when_thread_already_exists() {
        let mut state = BranchReviewState::default();
        let mut original = thread("t-a");
        original.is_resolved = false;
        original.line_number = 10;
        let mut updated = thread("t-a");
        updated.is_resolved = true;
        updated.line_number = 99;
        updated.start_line = 99;
        updated.end_line = 99;

        upsert_review_thread(&mut state, original);
        upsert_review_thread(&mut state, updated);

        assert_eq!(state.threads.len(), 1);
        assert_eq!(state.threads[0].line_number, 99);
        assert!(state.threads[0].is_resolved);
    }

    #[test]
    fn upsert_review_thread_preserves_insertion_order_for_different_ids() {
        let mut state = BranchReviewState::default();
        upsert_review_thread(&mut state, thread("t-1"));
        upsert_review_thread(&mut state, thread("t-2"));
        upsert_review_thread(&mut state, thread("t-3"));

        let ids: Vec<&str> = state.threads.iter().map(|t| t.id.as_str()).collect();
        assert_eq!(ids, vec!["t-1", "t-2", "t-3"]);
    }

    #[test]
    fn set_review_thread_resolved_true_marks_thread_resolved() {
        let mut state = BranchReviewState::default();
        let mut r1 = thread("r-1");
        r1.is_resolved = false;
        add_review_thread(&mut state, r1);
        set_review_thread_resolved(&mut state, "r-1", true);
        assert!(state.threads[0].is_resolved);
    }

    #[test]
    fn set_review_thread_resolved_false_reopens_a_resolved_thread() {
        let mut state = BranchReviewState::default();
        let mut r2 = thread("r-2");
        r2.is_resolved = true;
        add_review_thread(&mut state, r2);
        set_review_thread_resolved(&mut state, "r-2", false);
        assert!(!state.threads[0].is_resolved);
    }

    #[test]
    fn set_review_thread_resolved_is_a_no_op_for_unknown_ids() {
        let mut state = BranchReviewState::default();
        let mut r3 = thread("r-3");
        r3.is_resolved = false;
        add_review_thread(&mut state, r3);
        set_review_thread_resolved(&mut state, "no-such-id", true);
        assert!(!state.threads[0].is_resolved);
    }

    #[test]
    fn review_thread_accepts_side_old_and_new() {
        let mut state = BranchReviewState::default();
        let mut t_old = thread("side-old");
        t_old.side = ThreadSide::Old;
        let t_new = thread("side-new");
        add_review_thread(&mut state, t_old);
        add_review_thread(&mut state, t_new);
        assert_eq!(state.threads[0].side, ThreadSide::Old);
        assert_eq!(state.threads[1].side, ThreadSide::New);
    }

    // Note: the TS suite's "resolveReviewThread (legacy) still sets isResolved
    // to true" case is not ported — resolveReviewThread itself is not ported;
    // see the module doc. The behaviour it exercised is covered above by
    // set_review_thread_resolved_true_marks_thread_resolved.

    // --- new: not exercised by the TS suite ---

    #[test]
    fn set_branch_review_description_overwrites_the_field() {
        let mut state = BranchReviewState::default();
        set_branch_review_description(&mut state, "a review".to_string());
        assert_eq!(state.description, "a review");
    }

    #[test]
    fn set_branch_review_merge_strategy_overwrites_the_field() {
        let mut state = BranchReviewState::default();
        set_branch_review_merge_strategy(&mut state, MergeStrategy::Squash);
        assert_eq!(state.merge_strategy, MergeStrategy::Squash);
    }

    #[test]
    fn set_branch_review_diff_status_overwrites_the_field() {
        let mut state = BranchReviewState::default();
        set_branch_review_diff_status(&mut state, DiffStatus::Loading);
        assert_eq!(state.diff_status, DiffStatus::Loading);
    }

    #[test]
    fn add_review_thread_appends_to_the_end() {
        let mut state = BranchReviewState::default();
        add_review_thread(&mut state, thread("a"));
        add_review_thread(&mut state, thread("b"));
        let ids: Vec<&str> = state.threads.iter().map(|t| t.id.as_str()).collect();
        assert_eq!(ids, vec!["a", "b"]);
    }

    #[test]
    fn remove_review_thread_drops_only_the_matching_id() {
        let mut state = BranchReviewState::default();
        add_review_thread(&mut state, thread("a"));
        add_review_thread(&mut state, thread("b"));
        remove_review_thread(&mut state, "a");
        let ids: Vec<&str> = state.threads.iter().map(|t| t.id.as_str()).collect();
        assert_eq!(ids, vec!["b"]);
    }

    #[test]
    fn remove_review_thread_is_a_no_op_for_an_unknown_id() {
        let mut state = BranchReviewState::default();
        add_review_thread(&mut state, thread("a"));
        remove_review_thread(&mut state, "no-such-id");
        assert_eq!(state.threads.len(), 1);
    }

    #[test]
    fn add_review_message_appends_to_the_matching_thread() {
        let mut state = BranchReviewState::default();
        add_review_thread(&mut state, thread("t-1"));
        add_review_message(&mut state, "t-1", message("m-1", "hello"));
        assert_eq!(state.threads[0].messages.len(), 1);
        assert_eq!(state.threads[0].messages[0].body, "hello");
    }

    #[test]
    fn add_review_message_is_a_no_op_for_an_unknown_thread_id() {
        let mut state = BranchReviewState::default();
        add_review_thread(&mut state, thread("t-1"));
        add_review_message(&mut state, "no-such-id", message("m-1", "hello"));
        assert_eq!(state.threads[0].messages.len(), 0);
    }

    #[test]
    fn set_branch_review_conversations_replaces_the_whole_list() {
        let mut state = BranchReviewState::default();
        state.conversations.push(ReviewConversation {
            id: "old".to_string(),
            title: String::new(),
            age: String::new(),
            is_active: false,
        });
        set_branch_review_conversations(
            &mut state,
            vec![ReviewConversation {
                id: "new".to_string(),
                title: "t".to_string(),
                age: "1h".to_string(),
                is_active: true,
            }],
        );
        assert_eq!(state.conversations.len(), 1);
        assert_eq!(state.conversations[0].id, "new");
    }

    #[test]
    fn add_review_conversation_prepends_rather_than_appends() {
        let mut state = BranchReviewState::default();
        add_review_conversation(
            &mut state,
            ReviewConversation {
                id: "first".to_string(),
                title: String::new(),
                age: String::new(),
                is_active: false,
            },
        );
        add_review_conversation(
            &mut state,
            ReviewConversation {
                id: "second".to_string(),
                title: String::new(),
                age: String::new(),
                is_active: false,
            },
        );
        let ids: Vec<&str> = state.conversations.iter().map(|c| c.id.as_str()).collect();
        assert_eq!(ids, vec!["second", "first"]);
    }

    #[test]
    fn default_state_matches_initial_branch_review_state() {
        let state = BranchReviewState::default();
        assert_eq!(state.description, "");
        assert_eq!(state.merge_strategy, MergeStrategy::Merge);
        assert_eq!(state.diff_status, DiffStatus::Idle);
        assert!(state.threads.is_empty());
        assert!(state.conversations.is_empty());
    }
}
