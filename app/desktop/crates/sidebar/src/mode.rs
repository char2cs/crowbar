//! The three panels of the sidebar and which of them are usable.

use serde::{Deserialize, Serialize};

use crate::focus::Focus;

/// Chats, Files or Changes.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default, Serialize, Deserialize)]
pub enum Mode {
    /// The chats tree of the active space.
    #[default]
    Chats,
    /// The file tree of the focused workspace.
    Files,
    /// The changed files and merge box of the focused repository workspace.
    Changes,
}

impl Mode {
    /// Every mode in tab order.
    pub const ALL: [Self; 3] = [Self::Chats, Self::Files, Self::Changes];

    /// Position in the tab bar.
    pub fn index(self) -> usize {
        match self {
            Self::Chats => 0,
            Self::Files => 1,
            Self::Changes => 2,
        }
    }

    /// Whether the focused workspace context lets this mode be used: Files needs any workspace,
    /// Changes a repository workspace, Chats nothing.
    pub fn is_available(self, focus: Focus) -> bool {
        match self {
            Self::Chats => true,
            Self::Files => focus != Focus::Nothing,
            Self::Changes => focus == Focus::Repository,
        }
    }

    /// The stored choice when it is usable, Chats otherwise; the stored value is never rewritten.
    #[must_use]
    pub fn shown(self, focus: Focus) -> Self {
        if self.is_available(focus) {
            return self;
        }
        Self::Chats
    }

    /// Whether the tab is drawn at all: Changes disappears without a repository workspace.
    pub fn is_listed(self, focus: Focus) -> bool {
        self != Self::Changes || self.is_available(focus)
    }
}

#[cfg(test)]
#[path = "mode_tests.rs"]
mod tests;
