//! How a record's state reads on a sidebar row.

use daemonapi::Switch;
use sidebar::{Activity, Lock, Nesting, Titling};

use crate::live::WorkspaceRecord;
use crate::rows::forest::Tree;
use crate::rows::workspace_view;

pub(crate) const UNTITLED_CHAT: &str = "Untitled chat";
const GENERATED_BRANCH_PREFIX: &str = "chat-";
const GENERATED_BRANCH_DIGITS: usize = 8;

pub(crate) fn nesting_of(children: &[Tree]) -> Nesting {
    nesting_for(children.len())
}

pub(crate) fn nesting_for(children: usize) -> Nesting {
    if children == 0 {
        return Nesting::Leaf;
    }
    Nesting::Parent
}

pub(crate) fn lock_of(ws: &WorkspaceRecord) -> Lock {
    if workspace_view::is_locked(ws) {
        return Lock::Locked;
    }
    Lock::Unlocked
}

pub(crate) fn title_of(title: &str) -> &str {
    if title.is_empty() {
        return UNTITLED_CHAT;
    }
    title
}

pub(crate) fn titling_of(title: &str) -> Titling {
    if title.is_empty() {
        return Titling::Provisional;
    }
    Titling::Final
}

pub(crate) fn branch_titling_of(branch: &str) -> Titling {
    let generated = branch
        .strip_prefix(GENERATED_BRANCH_PREFIX)
        .is_some_and(|tail| {
            tail.len() == GENERATED_BRANCH_DIGITS
                && tail
                    .bytes()
                    .all(|byte| matches!(byte, b'0'..=b'9' | b'a'..=b'f'))
        });
    if generated {
        return Titling::Provisional;
    }
    Titling::Final
}

pub(crate) fn activity_of(working: Switch) -> Activity {
    if working.is_on() {
        return Activity::Working;
    }
    Activity::Idle
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn titles_activity_nesting_and_lock_read_from_the_record() {
        assert_eq!(title_of(""), UNTITLED_CHAT);
        assert_eq!(title_of("x"), "x");
        assert_eq!(titling_of(""), Titling::Provisional);
        assert_eq!(titling_of("x"), Titling::Final);
        assert_eq!(activity_of(Switch::On), Activity::Working);
        assert_eq!(activity_of(Switch::Off), Activity::Idle);
        assert_eq!(nesting_for(0), Nesting::Leaf);
        assert_eq!(nesting_for(2), Nesting::Parent);
        assert_eq!(lock_of(&WorkspaceRecord::default()), Lock::Unlocked);
    }

    #[test]
    fn only_a_branch_the_daemon_named_reads_provisional() {
        let cases = [
            ("chat-0a1b2c3d", Titling::Provisional),
            ("chat-0a1b2c3", Titling::Final),
            ("chat-0A1B2C3D", Titling::Final),
            ("feature/pricing-rounding", Titling::Final),
        ];
        for (branch, titling) in cases {
            assert_eq!(branch_titling_of(branch), titling, "{branch}");
        }
    }
}
