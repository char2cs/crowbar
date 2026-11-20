use crate::ide::pane_group::PaneGroup;

/// Whether a group's editor view shows beside its chat.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EditorView {
    /// The editor view shows, empty when the group has no editor tab.
    Open,
    /// Only the chat shows, unless the group has editor tabs.
    Closed,
}

impl EditorView {
    /// The state of `group`'s editor view.
    #[must_use]
    pub fn of(group: &PaneGroup) -> Self {
        if group.editor_open() {
            return Self::Open;
        }
        Self::Closed
    }

    /// The other state, as the open editor button and the split toggle flip it.
    #[must_use]
    pub fn flipped(self) -> Self {
        match self {
            Self::Open => Self::Closed,
            Self::Closed => Self::Open,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn flipping_twice_returns_to_the_start() {
        for view in [EditorView::Open, EditorView::Closed] {
            assert_ne!(view.flipped(), view, "{view:?} flips");
            assert_eq!(view.flipped().flipped(), view, "{view:?} back");
        }
    }
}
