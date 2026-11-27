use serde::{Deserialize, Serialize};

use crate::shell::Focus;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default, Serialize, Deserialize)]
pub enum Mode {
    #[default]
    Chats,
    Files,
    Changes,
}

impl Mode {
    pub const ALL: [Self; 3] = [Self::Chats, Self::Files, Self::Changes];

    pub fn index(self) -> usize {
        match self {
            Self::Chats => 0,
            Self::Files => 1,
            Self::Changes => 2,
        }
    }

    pub fn is_available(
        self,
        focus: Focus,
    ) -> bool {
        match self {
            Self::Chats => true,
            Self::Files => focus != Focus::Nothing,
            Self::Changes => focus == Focus::Repository,
        }
    }

    #[must_use]
    pub fn shown(
        self,
        focus: Focus,
    ) -> Self {
        if self.is_available(focus) {
            return self;
        }
        Self::Chats
    }

    pub fn is_listed(
        self,
        focus: Focus,
    ) -> bool {
        self != Self::Changes || self.is_available(focus)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn availability_follows_the_focused_workspace() {
        let cases = [
            (Mode::Chats, Focus::Nothing, true),
            (Mode::Files, Focus::Nothing, false),
            (Mode::Files, Focus::Workspace, true),
            (Mode::Changes, Focus::Workspace, false),
            (Mode::Changes, Focus::Repository, true),
        ];
        for (mode, focus, expected) in cases {
            assert_eq!(
                mode.is_available(focus),
                expected,
                "{mode:?} with {focus:?}"
            );
        }
    }

    #[test]
    fn an_unavailable_choice_falls_back_to_chats_without_being_forgotten() {
        let chosen = Mode::Changes;
        assert_eq!(chosen.shown(Focus::Nothing), Mode::Chats, "fallback");
        assert_eq!(
            chosen.shown(Focus::Repository),
            Mode::Changes,
            "back when available"
        );
    }

    #[test]
    fn only_changes_is_hidden_when_unavailable() {
        assert!(!Mode::Changes.is_listed(Focus::Workspace), "changes hidden");
        assert!(
            Mode::Files.is_listed(Focus::Nothing),
            "files stays listed, disabled"
        );
    }
}
