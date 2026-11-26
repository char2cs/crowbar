//! Whether a chat has a branch to review, as develop's tab row decides it.

use crate::live::State;
use crate::rows::target::{Kind, resolve};

pub(crate) fn of(
    state: &State,
    chat: &str,
) -> bool {
    resolve(state, chat).is_some_and(|target| owns_worktree(&target.kind))
}

fn owns_worktree(kind: &Kind) -> bool {
    matches!(kind, Kind::RepoHeader { .. } | Kind::Branch { .. })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn only_a_chat_that_owns_its_worktree_has_a_branch_to_review() {
        let (state, _) = state();
        let cases = [
            ("the repository's own checkout", "rdo", true),
            ("a branch's owner", "bo", true),
            ("a thread", "t", false),
            ("a project home chat", "hc", false),
            ("an unknown chat", "nope", false),
        ];
        for (name, chat, expected) in cases {
            assert_eq!(of(&state, chat), expected, "{name}");
        }
    }
}
