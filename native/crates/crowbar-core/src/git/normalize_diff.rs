//! What `web/src/features/git/utils/normalize-diff.ts` defended against, and
//! why this module ports the *invariant* rather than the two functions.
//!
//! # The original bug
//!
//! `GitDiff.lines` is declared non-nullable in TS (`lines: GitDiffLine[]`),
//! and roughly a dozen call sites dereference it directly
//! (`diff.lines.length`, `for (const line of diff.lines)`). The API used to
//! be able to send `lines: null` for some diffs (the TS source's own comment
//! names binary files); a fix on the daemon side stopped that
//! (`FileDiff.MarshalJSON`). The TS source's own comment says an **opened
//! diff tab is persisted with its payload embedded**, so a tab opened before
//! that fix would keep reappearing with the stale `lines: null` shape on
//! every relaunch — there is no refetch to correct it, only a `JSON.parse`
//! of whatever was written to disk. `normalizeGitDiff` / `normalizeMultiFileDiff`
//! existed to catch that one field, on every read, forever — a graceful
//! fallback (stale value reads as empty) rather than a migration, per this
//! project's own stated preference for pre-production code.
//!
//! **Checked, not assumed, and worth stating precisely:** the currently
//! persisted shape (`web/src/lib/persistence/schemas.ts`'s
//! `WorkspaceLayout.buffers: PaneContent[]`) does not actually embed a raw
//! `GitDiff` anywhere today — `web/src/features/panes/types/pane-content.ts`'s
//! diff-carrying variants, `CommitDiffContent`/`BranchReviewContent`, persist
//! only an id (`sha`/`wsId`) and fetch the diff body lazily per the viewport,
//! by their own doc comments. So the *precondition* the TS comment describes
//! does not appear to be reachable in the app as it stands today either —
//! consistent with, and probably the reason for, `normalize-diff.ts` having
//! zero production importers (§ below). This port cannot rule out an older
//! persisted-tab shape or a code path this search missed; it states what was
//! actually found rather than assuming the TS comment's history still holds.
//!
//! # Why this doesn't port as two functions
//!
//! Two independent facts remove the bug's entire precondition in the native
//! port, not just its symptom:
//!
//! 1. **The field can't lie about its own shape.** TS's `lines:
//!    GitDiffLine[]` is a compile-time-only promise — nothing stops a `null`
//!    from being cast through it at runtime, which is exactly how the bug
//!    happened. Rust's `Vec<T>` carries no such gap: a `Vec` value in memory
//!    is never null, by construction, for any Rust code that holds one —
//!    hand-written, generated, or deserialized. There is no runtime state a
//!    `normalize` function could observe and repair that the type doesn't
//!    already rule out.
//! 2. **The wire boundary already degrades gracefully, unconditionally.**
//!    `crowbar_proto::domain_git::FileDiff.lines` (and `MultiFileDiff.files`)
//!    deserialize through
//!    [`crowbar_proto::null_default::null_to_default`], which maps a JSON
//!    `null` onto the type's default (`vec![]`) rather than failing. Unlike
//!    the TS history, there was never a version of this port's `FileDiff`
//!    without that guard — `crowbar-proto` is generated fresh from the
//!    current Go source (§9.2), so there is no "opened before the fix" era
//!    of persisted payloads to defend against retroactively. This crate's
//!    tests below exist to prove that guard is real and to catch a
//!    regression in it, since `crowbar-core`'s callers rely on it just as
//!    much as the TS app relied on `normalizeGitDiff`.
//!
//! A function that always returns its input unchanged, backed by a test that
//! can only ever assert that unconditional fact, is exactly the
//! "declaration, not behaviour" shape this port's brief asks to be caught
//! rather than shipped (`crate::workspace::scope`'s test module documents
//! the same call on a dropped test case). So there is no
//! `normalize_git_diff`/`normalize_multi_file_diff` here — the two exports
//! `normalize-diff.ts` declares. What's ported instead is the regression
//! test that proves the invariant those two functions used to enforce by
//! hand is now enforced structurally, one layer down, for free.
//!
//! `utils/normalize-diff.ts` also has **no production importer** anywhere in
//! the current `web/src` tree (only its own test file does) —
//! `git grep`-confirmed independently of the `lib/persistence/` deletion
//! (§5.4) that would have removed its one real use case anyway.

#[cfg(test)]
mod tests {
    use crowbar_proto::domain_git::{FileDiff, MultiFileDiff};

    // --- proves the guard `normalize-diff.ts` used to provide by hand is
    // now provided by crowbar-proto's generated Deserialize impl ---
    //
    // This is the closest faithful reproduction of the TS bug available: the
    // exact wire shape a stale pre-fix persisted payload had
    // (`lines: null`), fed through the same deserialization path any
    // persisted-cache read would use. See `mutation testing` note below for
    // how this was confirmed to be a real, failable assertion and not a
    // restatement of the type signature.

    const BINARY_FILE_WITH_NULL_LINES: &str = r#"{
        "file_path": "assets/blob.bin",
        "is_new": true,
        "is_deleted": false,
        "is_renamed": false,
        "is_binary": true,
        "lines": null,
        "additions": 0,
        "deletions": 0,
        "uncommitted": false
    }"#;

    #[test]
    fn a_file_diff_with_a_literal_null_lines_field_deserializes_to_an_empty_vec() {
        let diff: FileDiff =
            serde_json::from_str(BINARY_FILE_WITH_NULL_LINES).expect("degrades, does not error");
        assert_eq!(diff.lines, Vec::new());
        assert_eq!(diff.file_path, "assets/blob.bin");
        assert!(diff.is_new);
    }

    #[test]
    fn a_multi_file_diff_with_no_files_array_at_all_deserializes_to_empty() {
        let json = r#"{
            "totalFiles": 0,
            "totalAdditions": 0,
            "totalDeletions": 0,
            "files": null
        }"#;
        let multi: MultiFileDiff = serde_json::from_str(json).expect("degrades, does not error");
        assert_eq!(multi.files, Vec::new());
    }

    #[test]
    fn a_multi_file_diff_carrying_a_null_lines_entry_degrades_only_that_entry() {
        // Mirrors normalize-diff.test.ts's "repairs only the broken entries
        // and keeps the good ones" case. In the TS source this needed a
        // second pass after parsing to fix the one broken entry; here both
        // entries are already well-formed the moment deserialization
        // succeeds, because the same per-field guard runs on every element
        // of the `files` array, not just the top level.
        let good = r#"{
            "file_path": "src/a.ts",
            "is_new": false,
            "is_deleted": false,
            "is_renamed": false,
            "lines": [{"line_type": "added", "content": "x"}],
            "additions": 1,
            "deletions": 0,
            "uncommitted": false
        }"#;
        let json = format!(
            r#"{{"totalFiles":2,"totalAdditions":1,"totalDeletions":0,"files":[{good},{BINARY_FILE_WITH_NULL_LINES}]}}"#,
        );
        let multi: MultiFileDiff = serde_json::from_str(&json).expect("degrades, does not error");
        assert_eq!(multi.files.len(), 2);
        assert_eq!(multi.files[0].lines.len(), 1);
        assert_eq!(multi.files[1].lines, Vec::new());
    }

    // --- mutation testing note (per this port's verification requirement) ---
    //
    // To confirm the tests above can actually fail rather than merely
    // restate the type signature, `crowbar-proto/src/generated/null_default.rs`'s
    // `null_to_default` was temporarily changed from:
    //
    //   Ok(Option::<T>::deserialize(deserializer)?.unwrap_or_default())
    //
    // to a direct pass-through that does not treat `null` specially:
    //
    //   T::deserialize(deserializer)
    //
    // `cargo test -p crowbar-core --lib git::normalize_diff` against that
    // mutation produced (real output, all three tests in this module,
    // pasted verbatim):
    //
    //   running 3 tests
    //   test git::normalize_diff::tests::a_file_diff_with_a_literal_null_lines_field_deserializes_to_an_empty_vec ... FAILED
    //   test git::normalize_diff::tests::a_multi_file_diff_carrying_a_null_lines_entry_degrades_only_that_entry ... FAILED
    //   test git::normalize_diff::tests::a_multi_file_diff_with_no_files_array_at_all_deserializes_to_empty ... FAILED
    //
    //   ---- git::normalize_diff::tests::a_file_diff_with_a_literal_null_lines_field_deserializes_to_an_empty_vec stdout ----
    //   thread '...' panicked at crates/crowbar-core/src/git/normalize_diff.rs:91:63:
    //   degrades, does not error: Error("invalid type: null, expected a sequence", line: 7, column: 21)
    //
    //   ---- git::normalize_diff::tests::a_multi_file_diff_carrying_a_null_lines_entry_degrades_only_that_entry stdout ----
    //   thread '...' panicked at crates/crowbar-core/src/git/normalize_diff.rs:130:64:
    //   degrades, does not error: Error("invalid type: null, expected a sequence", line: 16, column: 21)
    //
    //   ---- git::normalize_diff::tests::a_multi_file_diff_with_no_files_array_at_all_deserializes_to_empty stdout ----
    //   thread '...' panicked at crates/crowbar-core/src/git/normalize_diff.rs:105:63:
    //   degrades, does not error: Error("invalid type: null, expected a sequence", line: 5, column: 25)
    //
    //   test result: FAILED. 0 passed; 3 failed; 0 ignored; 0 measured; 121 filtered out
    //
    // i.e. the exact TS failure mode this file exists to document — a
    // malformed diff object propagating instead of degrading — reappears the
    // moment the guard is removed, and all three tests here catch it. The
    // mutation was reverted immediately after (confirmed via `git diff`/`git
    // status` on `crowbar-proto/` showing no changes) — it is a generated
    // file this item does not otherwise touch.
}
