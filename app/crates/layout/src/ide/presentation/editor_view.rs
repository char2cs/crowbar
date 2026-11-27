use crate::ide::pane_group::PaneGroup;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EditorView {
    Open,
    Closed,
}

impl EditorView {
    #[must_use]
    pub fn of(group: &PaneGroup) -> Self {
        if group.editor_open() {
            return Self::Open;
        }
        Self::Closed
    }

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
