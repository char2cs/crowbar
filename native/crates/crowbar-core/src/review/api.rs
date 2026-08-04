//! Wire-shape mappers for the branch-review REST client.
//!
//! Ported from `web/src/features/git/api/review-api.ts` (288 lines), **only**
//! the two pairs `native/mapping/tier-a-denominator.md` §7 names: `mapThread`
//! together with its private `mapReply` helper ([`map_thread`]/`map_reply`),
//! and `getReview` together with its private `mapConversation` helper
//! ([`build_review_state`]/`map_conversation`). The file's other 16 exports —
//! `listThreads`, the 9 CRUD/transport functions (`getReviewFiles`,
//! `mergeIntoParent`, `setMergeStrategy`, `openThread`, `replyToThread`,
//! `setThreadResolved`, `deleteThread`, `deleteMessage`, `editMessage`), and
//! 6 supporting DTO types — are transport, not model, and go to
//! `crowbar-client`, per this document's own convention. Not ported here.
//!
//! # `getReview` is split; `build_review_state` is the pure half
//!
//! TS `getReview(wsId: string): Promise<ReviewState>` does two things in one
//! function body: an `apiFetch` HTTP call, and a pure reshape of the response
//! into `ReviewState`. `crowbar-core` does no I/O (it has to stay testable
//! without a window), so this port keeps only the reshape — [`build_review_state`]
//! takes the already-fetched wire payload ([`WireBranchReview`]) and returns
//! [`ReviewState`]. The fetch itself belongs wherever `crowbar-client`'s
//! transport code eventually lands, following this file's own "transport,
//! not model" rule applied to `getReview`'s own body, not just its 16
//! siblings.
//!
//! One consequence of that split worth stating plainly: because
//! `crowbar-core` never runs the fetch, [`WireBranchReview`]/[`WireBranchChat`]
//! have to be `pub` here, unlike their TS counterparts, which are
//! module-private (`getReview` is the only thing in `review-api.ts` that
//! touches them, because it also does the fetch). A future transport
//! function needs a public wire type to construct from the deserialised
//! response before it can call [`build_review_state`].
//!
//! # A field dropped, and a field kept-but-always-empty — checked against the Go backend, not assumed
//!
//! TS `WireBranchReview`/`ReviewState` both carry a `diff: MultiFileDiff`
//! field. It is **not** ported here. Two independent facts, both confirmed by
//! reading the actual source rather than inferred from the TS types:
//!
//! - **The backend never sends it.** `api/internal/domain/branch_review.go`'s
//!   `BranchReview` struct is `{Description, MergeStrategy, Threads}` —
//!   no diff field at all — and `api/internal/api/v0/dto/review.go`'s
//!   `BranchReviewDTO` (what `GET /v0/workspaces/:wsId/review` actually
//!   serialises, via `BranchReviewDTOFrom`) matches it exactly. `raw.diff` in
//!   the TS source is `undefined` on every real response despite its
//!   non-optional `MultiFileDiff` type.
//! - **The frontend never stores it.** `branch-review-pane.tsx`'s own comment,
//!   at its one call site: *"Deliberately NOT storing review.diff. The pane
//!   renders from the files summary + outline now; keeping the composite's
//!   line-level diff would re-import the very payload this phase removed."*
//!
//! A field with no producer and no consumer on either side of the wire is
//! exactly the "declaration, not behaviour" shape `crate::git`'s module doc
//! already declines to port for `types::GitDiff`'s six unused `FileDiff`
//! fields — same rule, independently re-confirmed here.
//!
//! `conversations` is a milder version of the same story, and **is** kept
//! (per §7's explicit "port each pair as one unit" scope, and because
//! `super::state`'s `BranchReviewState::conversations`/
//! `set_branch_review_conversations`/`add_review_conversation` are
//! independently LIVE store surface this item ports regardless): the backend
//! *used* to serialise a `conversations` key, but a regression test —
//! `api/internal/api/v0/dto/review_test.go`'s
//! `TestBranchReviewDTOFromHasNoConversationsKey`, whose own comment reads
//! *"guards the Chat-aggregate removal (decision 2): the branch-review wire
//! model must no longer carry a `conversations` key. The frontend degrades to
//! `raw.conversations ?? []`, so its absence is FE-safe"* — proves the
//! backend deliberately stopped sending it. So [`map_conversation`] is
//! reachable in this port exactly as it is in the TS source (through
//! [`build_review_state`]'s optional-field handling), but nothing in the
//! *current* backend contract can ever hand it a real value — this was not
//! caught by `tier-a-denominator.md`'s export-liveness method (which traces
//! frontend reachability, not backend wire truth) and is recorded here as a
//! finding, not silently pruned, since the brief's scope was explicit and the
//! store fields it feeds remain genuinely live.

use crowbar_proto::api_v0_dto::{ThreadDTO, ThreadReplyDTO};
use crowbar_proto::domain_git::MergeStrategy;

use super::state::{ReviewConversation, ReviewMessage, ReviewThread, ThreadSide};

/// The wire's `side` field is a plain `String` on the generated [`ThreadDTO`]
/// — the binding generator could not see that the Go/TS side of the wire
/// only ever sends `"old"` or `"new"`; TS's own `ThreadDTO.side: 'old' | 'new'`
/// is a compile-time-only promise `mapThread` never checks at runtime, it
/// just copies `t.side` straight through. This makes that promise explicit
/// at the one point it crosses the wire: anything other than the literal
/// `"old"` becomes [`ThreadSide::New`], mirroring `toAnnotationSide`'s own
/// `side === 'old' ? 'deletions' : 'additions'` — the one place the original
/// source reveals what "anything else" was already assumed to mean.
fn parse_side(side: &str) -> ThreadSide {
    if side == "old" {
        ThreadSide::Old
    } else {
        ThreadSide::New
    }
}

/// Maps one reply DTO to a plain message. Mirrors the module-private
/// `mapReply`; kept private here too — unreachable except through
/// [`map_thread`], matching the TS source.
fn map_reply(r: &ThreadReplyDTO) -> ReviewMessage {
    ReviewMessage {
        id: r.id.clone(),
        author: if r.author.is_empty() { None } else { Some(r.author.clone()) },
        is_agent: r.is_agent,
        body: r.body.clone(),
        created_at: r.created_at.clone(),
    }
}

/// Maps a backend [`ThreadDTO`] (from `/threads` or the WS stream) to the
/// store's [`ReviewThread`]. The root comment body/author/isAgent live at
/// the top level of `ThreadDTO`; subsequent messages are in `replies[]`.
/// Mirrors `mapThread`.
#[must_use]
pub fn map_thread(t: &ThreadDTO) -> ReviewThread {
    let root_message = ReviewMessage {
        // Prefer the root comment's real id (so it can be edited via
        // /messages/:id); fall back to the synthetic id for any
        // pre-`messageId` payloads.
        id: if t.message_id.is_empty() {
            format!("{}:root", t.id)
        } else {
            t.message_id.clone()
        },
        author: if t.author.is_empty() { None } else { Some(t.author.clone()) },
        is_agent: t.is_agent,
        body: t.body.clone(),
        created_at: t.created_at.clone(),
    };

    let mut messages = Vec::with_capacity(1 + t.replies.len());
    messages.push(root_message);
    messages.extend(t.replies.iter().map(map_reply));

    ReviewThread {
        id: t.id.clone(),
        file_path: t.file_path.clone(),
        line_number: t.line,
        // Prefer startLine/endLine, else fall back to line for both — mirrors
        // TS `t.startLine || t.line` (JS `||` falls back only on the literal
        // falsy `0`, never on a negative number, which `== 0` matches).
        start_line: if t.start_line == 0 { t.line } else { t.start_line },
        end_line: if t.end_line == 0 { t.line } else { t.end_line },
        side: parse_side(&t.side),
        messages,
        is_resolved: t.resolved,
    }
}

/// The composite branch-review read model, reshaped from the wire. Mirrors
/// TS `ReviewState`. `diff` is not a field here — see the module doc.
#[derive(Debug, Clone, PartialEq)]
pub struct ReviewState {
    pub description: String,
    pub merge_strategy: MergeStrategy,
    /// Always empty — threads are sourced exclusively from `/threads` + the
    /// WS stream, never from this composite endpoint. Mirrors `getReview`'s
    /// own `threads: []` and its doc comment stating the same.
    pub threads: Vec<ReviewThread>,
    pub conversations: Vec<ReviewConversation>,
}

/// One entry in the (see module doc: currently never populated)
/// conversations list, as read off the wire. Mirrors the TS private
/// interface `WireBranchChat`.
#[derive(Debug, Clone, PartialEq, Default)]
pub struct WireBranchChat {
    pub id: String,
    pub title: Option<String>,
    pub age: Option<String>,
    pub is_active: Option<bool>,
}

/// Maps one wire conversation entry to the store's [`ReviewConversation`].
/// Mirrors the module-private `mapConversation`; kept private here too —
/// unreachable except through [`build_review_state`], matching the TS
/// source.
fn map_conversation(c: &WireBranchChat) -> ReviewConversation {
    ReviewConversation {
        id: c.id.clone(),
        title: c.title.clone().unwrap_or_default(),
        age: c.age.clone().unwrap_or_default(),
        is_active: c.is_active.unwrap_or(false),
    }
}

/// The raw composite read model, as read off the wire, before reshaping.
/// Mirrors the TS private interface `WireBranchReview` — minus `diff` and
/// minus the (already-ignored, see TS's own comment) `threads` field; see
/// the module doc for why. `pub` for the reason given there too.
///
/// No `#[derive(Default)]`: [`MergeStrategy`] is a foreign
/// (`crowbar-proto`-generated) type with no `Default` impl of its own, and
/// the orphan rule means this crate cannot add one — a construct-with-`..`
/// shorthand is not available for this struct in tests; every field is
/// spelled out instead.
#[derive(Debug, Clone, PartialEq)]
pub struct WireBranchReview {
    pub description: String,
    pub merge_strategy: MergeStrategy,
    pub conversations: Option<Vec<WireBranchChat>>,
}

/// Reshapes the wire's composite branch-review read model into
/// [`ReviewState`]. This is `getReview`'s pure half — see the module doc for
/// what was left out (the `apiFetch` call itself) and why.
#[must_use]
pub fn build_review_state(raw: &WireBranchReview) -> ReviewState {
    ReviewState {
        description: raw.description.clone(),
        merge_strategy: raw.merge_strategy.clone(),
        threads: Vec::new(),
        conversations: raw
            .conversations
            .as_deref()
            .unwrap_or(&[])
            .iter()
            .map(map_conversation)
            .collect(),
    }
}

#[cfg(test)]
mod tests {
    use super::{ReviewState, WireBranchChat, WireBranchReview, build_review_state, map_thread};
    use crowbar_proto::api_v0_dto::{ThreadDTO, ThreadReplyDTO};
    use crowbar_proto::domain_git::MergeStrategy;

    fn wire_thread_dto() -> ThreadDTO {
        ThreadDTO {
            id: "t1".to_string(),
            project_id: "p1".to_string(),
            repo_id: "r1".to_string(),
            workspace_id: "ws-1".to_string(),
            file_path: "README.md".to_string(),
            line: 10,
            start_line: 8,
            end_line: 12,
            side: "new".to_string(),
            message_id: "m0".to_string(),
            body: "root comment".to_string(),
            author: "char2cs".to_string(),
            is_agent: false,
            resolved: false,
            created_at: "2026-01-01T00:00:00Z".to_string(),
            replies: Vec::new(),
            deleted: None,
        }
    }

    fn wire_reply_dto() -> ThreadReplyDTO {
        ThreadReplyDTO {
            id: "r1".to_string(),
            thread_id: "thread-1".to_string(),
            body: "reply".to_string(),
            author: "claude".to_string(),
            is_agent: true,
            created_at: "2026-06-18T01:00:00Z".to_string(),
        }
    }

    // --- ported from web/src/__tests__/features/git/api/review-api.test.ts ---

    #[test]
    fn maps_real_thread_dto_fields_line_to_line_number_resolved_to_is_resolved_body_and_replies_to_messages()
     {
        let dto = ThreadDTO {
            id: "thread-1".to_string(),
            line: 7,
            start_line: 7,
            end_line: 8,
            side: "new".to_string(),
            message_id: "m0".to_string(),
            body: "hi **x**".to_string(),
            author: "char2cs".to_string(),
            is_agent: false,
            resolved: false,
            created_at: "2026-06-18T00:00:00Z".to_string(),
            replies: vec![wire_reply_dto()],
            ..wire_thread_dto()
        };

        let result = map_thread(&dto);

        assert_eq!(result.line_number, 7);
        assert_eq!(result.start_line, 7);
        assert_eq!(result.end_line, 8);
        assert_eq!(result.side, super::super::state::ThreadSide::New);
        assert!(!result.is_resolved);
        assert_eq!(result.messages.len(), 2);
        assert_eq!(result.messages[0].body, "hi **x**");
        assert_eq!(result.messages[0].author.as_deref(), Some("char2cs"));
        assert!(!result.messages[0].is_agent);
        assert_eq!(result.messages[1].id, "r1");
        assert_eq!(result.messages[1].body, "reply");
        assert_eq!(result.messages[1].author.as_deref(), Some("claude"));
        assert!(result.messages[1].is_agent);
    }

    #[test]
    fn derives_is_resolved_from_resolved_true() {
        let dto = ThreadDTO { resolved: true, ..wire_thread_dto() };
        assert!(map_thread(&dto).is_resolved);
    }

    #[test]
    fn derives_is_resolved_false_from_resolved_false() {
        let dto = ThreadDTO { resolved: false, ..wire_thread_dto() };
        assert!(!map_thread(&dto).is_resolved);
    }

    #[test]
    fn falls_back_start_line_end_line_to_line_when_zero() {
        let dto = ThreadDTO { line: 5, start_line: 0, end_line: 0, ..wire_thread_dto() };
        let result = map_thread(&dto);
        assert_eq!(result.line_number, 5);
        assert_eq!(result.start_line, 5);
        assert_eq!(result.end_line, 5);
    }

    #[test]
    fn root_message_uses_the_real_message_id_from_the_wire() {
        let dto = ThreadDTO { id: "abc".to_string(), message_id: "root-real".to_string(), ..wire_thread_dto() };
        assert_eq!(map_thread(&dto).messages[0].id, "root-real");
    }

    #[test]
    fn root_message_falls_back_to_synthetic_id_root_when_message_id_is_empty() {
        let dto = ThreadDTO { id: "abc".to_string(), message_id: String::new(), ..wire_thread_dto() };
        assert_eq!(map_thread(&dto).messages[0].id, "abc:root");
    }

    #[test]
    fn empty_replies_produces_a_single_root_message() {
        let dto = ThreadDTO { replies: Vec::new(), ..wire_thread_dto() };
        assert_eq!(map_thread(&dto).messages.len(), 1);
    }

    #[test]
    fn maps_side_correctly() {
        let old = ThreadDTO { side: "old".to_string(), ..wire_thread_dto() };
        let new = ThreadDTO { side: "new".to_string(), ..wire_thread_dto() };
        assert_eq!(map_thread(&old).side, super::super::state::ThreadSide::Old);
        assert_eq!(map_thread(&new).side, super::super::state::ThreadSide::New);
    }

    #[test]
    fn get_review_reads_mapped_merge_strategy_and_returns_threads_empty() {
        // Adapted from review-api.test.ts's "getReview reads the
        // workspace-scoped /review route and returns threads:[]" — the
        // route/fetch half of that test is not ported (transport, not
        // model); only the mapping half is exercised here.
        let raw = WireBranchReview {
            description: "desc".to_string(),
            merge_strategy: MergeStrategy::Squash,
            // The old /review composite's `threads` field is intentionally
            // ignored by getReview itself, so it is not even a field on
            // WireBranchReview here — nothing to pass.
            conversations: None,
        };

        let review: ReviewState = build_review_state(&raw);

        assert_eq!(review.merge_strategy, MergeStrategy::Squash);
        assert_eq!(review.threads, Vec::new());
        assert_eq!(review.conversations, Vec::new());
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn maps_side_other_than_old_to_new() {
        // TS's ThreadDTO.side is typed 'old' | 'new' at compile time only;
        // the generated Rust wire type is a plain String. Exercises the
        // fallback this port had to make explicit — see parse_side's doc.
        let dto = ThreadDTO { side: "sideways".to_string(), ..wire_thread_dto() };
        assert_eq!(map_thread(&dto).side, super::super::state::ThreadSide::New);
    }

    #[test]
    fn build_review_state_passes_description_through_unchanged() {
        let raw = WireBranchReview {
            description: "a review".to_string(),
            merge_strategy: MergeStrategy::Merge,
            conversations: None,
        };
        assert_eq!(build_review_state(&raw).description, "a review");
    }

    #[test]
    fn build_review_state_maps_conversations_when_present() {
        let raw = WireBranchReview {
            description: String::new(),
            merge_strategy: MergeStrategy::Merge,
            conversations: Some(vec![
                WireBranchChat {
                    id: "c1".to_string(),
                    title: Some("Fix the header".to_string()),
                    age: Some("2h".to_string()),
                    is_active: Some(true),
                },
                WireBranchChat { id: "c2".to_string(), title: None, age: None, is_active: None },
            ]),
        };

        let review = build_review_state(&raw);

        assert_eq!(review.conversations.len(), 2);
        assert_eq!(review.conversations[0].id, "c1");
        assert_eq!(review.conversations[0].title, "Fix the header");
        assert_eq!(review.conversations[0].age, "2h");
        assert!(review.conversations[0].is_active);
        // Absent optional wire fields default the same way TS's `??`/`||`
        // fallbacks do in mapConversation.
        assert_eq!(review.conversations[1].title, "");
        assert_eq!(review.conversations[1].age, "");
        assert!(!review.conversations[1].is_active);
    }

    #[test]
    fn build_review_state_defaults_conversations_to_empty_when_absent() {
        let raw = WireBranchReview {
            description: String::new(),
            merge_strategy: MergeStrategy::Merge,
            conversations: None,
        };
        assert_eq!(build_review_state(&raw).conversations, Vec::new());
    }
}
