//! `web/src/features/file-system/controllers/file-utils.ts`'s
//! `getFileName`/`getFilenameFromPath`.
//!
//! The TS source declares these as one function under two names:
//! ```ts
//! export function getFileName(path: string): string {
//!   return path.split('/').pop() ?? path
//! }
//! /** Alias. */
//! export const getFilenameFromPath = getFileName
//! ```
//! `getFilenameFromPath` is a literal alias assignment, not a second
//! implementation — `native/mapping/tier-a-denominator.md` §8 confirms the
//! live call site (`editor-status-actions.tsx`'s always-mounted toolbar
//! project-name display) imports the alias spelling, but both names resolve
//! to the identical function. **Ported once, under the base name,
//! [`get_file_name`]** — there is nothing for a second Rust name to add over
//! a `use` of this one.

/// Mirrors `getFileName` (aka `getFilenameFromPath`): the last
/// `/`-separated segment of `path`, or `path` itself if it has none.
///
/// The `unwrap_or(path)` fallback mirrors the TS source's `?? path`, but is
/// unreachable in practice: `str::rsplit` on any input, including `""`,
/// always yields at least one item, so `.next()` is never `None`. Kept for
/// fidelity to the TS source's own defensive `??` (and because this crate
/// denies `unwrap`/`expect` outside tests, so `unwrap_or` is the idiomatic
/// spelling regardless) — expect a small, permanent, unreachable-branch
/// coverage gap here, the same shape this crate already tolerates elsewhere
/// (see `crate::settings`'s own coverage note on wildcard-arm bookkeeping).
#[must_use]
pub fn get_file_name(path: &str) -> &str {
    path.rsplit('/').next().unwrap_or(path)
}

#[cfg(test)]
mod tests {
    use super::get_file_name;

    // --- new: no dedicated TS test file for file-utils.ts; behaviour read
    //     directly from the two-line source above ---

    #[test]
    fn returns_the_last_path_segment() {
        assert_eq!(get_file_name("/workspace/src/main.rs"), "main.rs");
        assert_eq!(get_file_name("README.md"), "README.md");
    }

    #[test]
    fn returns_an_empty_string_for_a_path_ending_in_a_separator() {
        // 'a/b/'.split('/').pop() is '' (the segment after the trailing
        // slash), not the segment before it — this is NOT a directory-name
        // extractor, it's a literal last-segment split.
        assert_eq!(get_file_name("a/b/"), "");
    }

    #[test]
    fn returns_the_whole_string_when_there_is_no_separator() {
        assert_eq!(get_file_name("Makefile"), "Makefile");
    }

    #[test]
    fn returns_an_empty_string_for_an_empty_path() {
        assert_eq!(get_file_name(""), "");
    }
}
