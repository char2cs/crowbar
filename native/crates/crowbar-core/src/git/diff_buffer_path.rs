//! Parse the synthetic `diff://…` buffer-path scheme back to the real
//! on-disk file path it addresses, for editor-tab identity.
//!
//! Ported from `web/src/features/git/utils/diff-buffer-path.ts`. Three
//! virtual buffer shapes are recognised:
//!
//! * `diff://staged/<path>` / `diff://unstaged/<path>` — a single
//!   working-tree file's diff.
//! * `diff://commit/<hash>/<path>[.diff]` — one file of a commit's diff. A
//!   `.diff` extension is stripped if present (opened-as-a-file commit
//!   diffs carry it); `<path> == "all-files"` addresses the WHOLE commit,
//!   not a single file, and returns `None`.
//! * `diff://stash/<index>/<path>` — one file of a stash diff, same
//!   `"all-files"` exclusion.
//!
//! Anything else starting with `diff://` that matches none of these returns
//! `None` (an aggregate/unrecognised virtual buffer with no single file
//! path). Anything NOT starting with `diff://` is assumed to already be a
//! real path (e.g. an opened `.patch` file) and is returned unchanged.
//!
//! # Liveness note
//!
//! `getDiffBufferFilePath` has no production importer anywhere in the
//! current `web/src` tree — its only caller is
//! `web/src/__tests__/features/git/use-git-diff-data.test.ts`, and there is
//! no `use-git-diff-data.ts` implementation file for that test to actually
//! be testing a hook of (confirmed: `find web/src -iname
//! 'use-git-diff-data*'` returns only the test). Ported anyway per this
//! item's explicit scope (it is real, non-trivial parsing logic, unlike
//! `normalize_diff` — see that module's doc comment for the case where
//! dead-in-TS additionally means "cannot fail" in Rust).
//!
//! # The `.diff`-suffix edge case that isn't in any existing test
//!
//! The TS regex for the commit case is
//! `^diff:\/\/commit\/[^/]+\/(.+?)(?:\.diff)?$` — a **non-greedy** capture
//! followed by an *optional* literal suffix, anchored at the end. Worked
//! through by hand (no existing test exercises it): this is equivalent to
//! "strip a trailing `.diff` if present, unless doing so would leave the
//! capture empty" — `.+?` requires at least one captured character, so
//! `diff://commit/h/.diff` (bare filename `.diff`, nothing before it) must
//! NOT strip down to an empty string; the whole `.diff` stays.
//!
//! Confirmed with a real mutation, not just this derivation: temporarily
//! changed `strip_trailing_diff_suffix` to the naive
//! `s.strip_suffix(".diff").unwrap_or(s)` (dropping the "don't empty the
//! capture" filter) and ran
//! `cargo test -p crowbar-core --lib git::diff_buffer_path`. Result (real
//! output):
//!
//! ```text
//! test git::diff_buffer_path::tests::a_commit_file_literally_named_dot_diff_keeps_its_full_name ... FAILED
//! thread '...' panicked at crates/crowbar-core/src/git/diff_buffer_path.rs:228:9:
//! assertion `left == right` failed
//!   left: Some("")
//!  right: Some(".diff")
//! test result: FAILED. 10 passed; 1 failed; 0 ignored; 0 measured; 113 filtered out
//! ```
//!
//! The other ten tests in this module still passed, confirming the mutation
//! affected only this one edge case. Reverted immediately after (`git diff`
//! on this file showed only the one-line change, then restored).

/// Mirrors `getDiffBufferFilePath`.
#[must_use]
pub fn get_diff_buffer_file_path(buffer_path: Option<&str>) -> Option<String> {
    let buffer_path = buffer_path.filter(|s| !s.is_empty())?;

    if let Some(file_path) = working_tree_file_path(buffer_path) {
        return Some(decode_uri_component(file_path));
    }
    if let Some(file_path) = commit_file_path(buffer_path) {
        return Some(decode_uri_component(file_path));
    }
    if let Some(file_path) = stash_file_path(buffer_path) {
        return Some(decode_uri_component(file_path));
    }

    if !buffer_path.starts_with("diff://") {
        return Some(buffer_path.to_string());
    }

    None
}

/// `^diff:\/\/(staged|unstaged)\/(.+)$`.
fn working_tree_file_path(buffer_path: &str) -> Option<&str> {
    let rest = buffer_path
        .strip_prefix("diff://staged/")
        .or_else(|| buffer_path.strip_prefix("diff://unstaged/"))?;
    (!rest.is_empty()).then_some(rest)
}

/// `^diff:\/\/commit\/[^/]+\/(.+?)(?:\.diff)?$`, excluding `all-files`.
fn commit_file_path(buffer_path: &str) -> Option<&str> {
    let after_commit = buffer_path.strip_prefix("diff://commit/")?;
    let slash = after_commit.find('/')?;
    let (hash, rest) = after_commit.split_at(slash);
    let remainder = &rest[1..];
    if hash.is_empty() || remainder.is_empty() {
        return None;
    }
    let file_path = strip_trailing_diff_suffix(remainder);
    (file_path != "all-files").then_some(file_path)
}

/// `^diff:\/\/stash\/\d+\/(.+)$`, excluding `all-files`.
fn stash_file_path(buffer_path: &str) -> Option<&str> {
    let after_stash = buffer_path.strip_prefix("diff://stash/")?;
    let slash = after_stash.find('/')?;
    let (index, rest) = after_stash.split_at(slash);
    let remainder = &rest[1..];
    let is_index = !index.is_empty() && index.bytes().all(|b| b.is_ascii_digit());
    if !is_index || remainder.is_empty() || remainder == "all-files" {
        return None;
    }
    Some(remainder)
}

/// Strip one trailing literal `.diff`, unless the string IS exactly `.diff`
/// (nothing would be left to capture — see the module doc's regex
/// derivation).
fn strip_trailing_diff_suffix(s: &str) -> &str {
    s.strip_suffix(".diff")
        .filter(|stripped| !stripped.is_empty())
        .unwrap_or(s)
}

/// `decodeURIComponent`, restricted to what this scheme ever needs:
/// percent-decode `%XX` byte triplets, leaving everything else untouched,
/// then reassemble as UTF-8. `crowbar-core`'s dependency list is
/// `crowbar-proto`/`crowbar-client` only (§4.2) — same reasoning as
/// `workspace::scope_url`'s hand-rolled `encodeURIComponent` for not pulling
/// in a URL crate for one pass with no parsing.
///
/// Diverges from JS on malformed input: `decodeURIComponent` **throws** a
/// `URIError` on a truncated/invalid escape or a byte sequence that isn't
/// valid UTF-8 once decoded. `crowbar-core` functions must not panic outside
/// tests (§4.3 rule 4), and every `diff://` buffer path in the real app is
/// constructed by Crowbar's own code via `encodeURIComponent` — never
/// user-typed — so a malformed escape here would itself be a bug elsewhere.
/// This falls back to returning the input unchanged rather than panicking.
fn decode_uri_component(input: &str) -> String {
    let bytes = input.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'%'
            && i + 2 < bytes.len()
            && let (Some(hi), Some(lo)) = (hex_digit(bytes[i + 1]), hex_digit(bytes[i + 2]))
        {
            out.push((hi << 4) | lo);
            i += 3;
            continue;
        }
        out.push(bytes[i]);
        i += 1;
    }
    String::from_utf8(out).unwrap_or_else(|_| input.to_string())
}

fn hex_digit(b: u8) -> Option<u8> {
    match b {
        b'0'..=b'9' => Some(b - b'0'),
        b'a'..=b'f' => Some(b - b'a' + 10),
        b'A'..=b'F' => Some(b - b'A' + 10),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::get_diff_buffer_file_path;

    // --- ported from web/src/__tests__/features/git/use-git-diff-data.test.ts ---
    // (the only existing test file for this function — see the module doc's
    // liveness note)

    #[test]
    fn resolves_virtual_working_tree_diff_paths() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://unstaged/src%2Fapp.ts")),
            Some("src/app.ts".to_string())
        );
    }

    #[test]
    fn uses_real_diff_buffer_paths_for_opened_patch_files() {
        assert_eq!(
            get_diff_buffer_file_path(Some("/repo/fix.patch")),
            Some("/repo/fix.patch".to_string())
        );
    }

    #[test]
    fn keeps_aggregate_virtual_diff_buffers_without_a_single_file_path() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://commit/abc123/all-files")),
            None
        );
    }

    // --- new: the other two schemes and edge cases, not exercised upstream ---

    #[test]
    fn resolves_a_staged_diff_path() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://staged/src/app.ts")),
            Some("src/app.ts".to_string())
        );
    }

    #[test]
    fn resolves_a_stash_diff_path() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://stash/2/src/app.ts")),
            Some("src/app.ts".to_string())
        );
    }

    #[test]
    fn a_non_numeric_stash_index_does_not_match() {
        assert_eq!(get_diff_buffer_file_path(Some("diff://stash/x/a.ts")), None);
    }

    #[test]
    fn a_stash_all_files_buffer_has_no_single_file_path() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://stash/0/all-files")),
            None
        );
    }

    #[test]
    fn strips_a_dot_diff_extension_from_an_opened_commit_file() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://commit/abc123/src/app.ts.diff")),
            Some("src/app.ts".to_string())
        );
    }

    #[test]
    fn a_commit_file_literally_named_dot_diff_keeps_its_full_name() {
        // The regex's capture group requires at least one character before
        // considering the optional ".diff" suffix, so a bare ".diff" (the
        // whole remainder, nothing to strip down to) is NOT emptied out.
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://commit/abc123/.diff")),
            Some(".diff".to_string())
        );
    }

    #[test]
    fn none_for_empty_or_missing_buffer_path() {
        assert_eq!(get_diff_buffer_file_path(Some("")), None);
        assert_eq!(get_diff_buffer_file_path(None), None);
    }

    #[test]
    fn none_for_an_unrecognised_diff_scheme() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://something-else/x")),
            None
        );
    }

    #[test]
    fn an_empty_commit_hash_does_not_match() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://commit//src/app.ts")),
            None
        );
    }

    #[test]
    fn decodes_a_lowercase_percent_escape() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://unstaged/src%2fapp.ts")),
            Some("src/app.ts".to_string())
        );
    }

    #[test]
    fn an_invalid_percent_escape_is_left_untouched_rather_than_erroring() {
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://unstaged/src%zzapp.ts")),
            Some("src%zzapp.ts".to_string())
        );
    }

    #[test]
    fn a_commit_buffer_path_with_no_file_segment_at_all_does_not_match() {
        // No second "/" after the hash: `[^/]+\/` in the source regex has
        // nothing to anchor to.
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://commit/abc123")),
            None
        );
    }

    #[test]
    fn a_stash_buffer_path_with_no_file_segment_at_all_does_not_match() {
        assert_eq!(get_diff_buffer_file_path(Some("diff://stash/2")), None);
    }

    #[test]
    fn a_percent_escape_that_decodes_to_invalid_utf8_falls_back_to_the_raw_input() {
        // %FF is valid hex but 0xFF is never a legal UTF-8 byte on its own —
        // this exercises decode_uri_component's whole-string fallback, a
        // different path than an invalid escape (which leaves just the `%`
        // and its following characters untouched, byte by byte).
        assert_eq!(
            get_diff_buffer_file_path(Some("diff://unstaged/%FF")),
            Some("%FF".to_string())
        );
    }
}
