#![forbid(unsafe_code)]

//! `crowbar-core` — every piece of Crowbar logic that can be tested without a
//! window.
//!
//! Git model, diff algebra, keymap resolution, settings schema and
//! validation, file-tree model, workspace/path scoping and the review-thread
//! model all land here (spec §4.2). [`workspace`] was the first of these to
//! land, item P3.53. [`git`] was the second. [`keymap`] was the third.
//! [`settings`] is the fourth — see its module doc for what was ported and
//! what wasn't.
//!
//! Dependency contract (§4.2): `crowbar-proto`, `crowbar-client`.
//!
//! **This crate must never depend on `gpui` (§4.3 rule 1, D2).** If something
//! here appears to need `gpui`, it is not logic — split the rendering half out
//! into `crowbar-ui` or `crowbar-state` and leave the decision here.

pub mod color;
pub mod git;
pub mod keymap;
pub mod settings;
pub mod workspace;

#[cfg(test)]
mod tests {
    /// Proves the `clippy.toml` test exemptions from §4.3 rule 4 are actually
    /// in force. `unwrap` and `expect` are `deny` workspace-wide; if
    /// `allow-unwrap-in-tests` / `allow-expect-in-tests` ever stop being read
    /// — a moved `clippy.toml`, a renamed key, a `CLIPPY_CONF_DIR` override —
    /// `cargo clippy --workspace --all-targets -- -D warnings` fails here
    /// instead of quietly pushing every future test file into per-file
    /// `#[allow]`s that are indistinguishable from silencing shipping code.
    #[test]
    fn clippy_test_exemptions_are_live() {
        let parsed: u32 = "41".parse().unwrap();
        let one = "1".parse::<u32>().expect("a literal parses");
        assert_eq!(parsed + one, 42);
    }

    /// Same guard, for `allow-panic-in-tests`.
    #[test]
    #[should_panic(expected = "panic is exempt inside tests")]
    fn panic_is_permitted_in_tests() {
        panic!("panic is exempt inside tests");
    }
}
