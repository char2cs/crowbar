//! How a record's state reads on a sidebar row.

use daemonapi::Switch;
use sidebar::{Activity, Lock, Nesting, Titling};

use crate::live::WorkspaceRecord;
use crate::rows::forest::Tree;
use crate::rows::workspace_view;

pub(crate) const UNTITLED_CHAT: &str = "Untitled chat";

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
}
