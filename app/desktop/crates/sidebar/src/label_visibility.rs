//! Which tab labels of the mode bar are drawn.

const ACTIVE_LABEL_FROM: f32 = 280.0;
const ALL_LABELS_FROM: f32 = 420.0;
const BAR_PADDING: f32 = 16.0;

/// Labels hide as the sidebar narrows: all from 420px of bar, the active one from 280px.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LabelVisibility {
    /// Icons only.
    Hidden,
    /// Only the active tab shows its label.
    ActiveOnly,
    /// Every tab shows its label.
    All,
}

impl LabelVisibility {
    /// For a sidebar `sidebar_width` pixels wide; the bar is 16px narrower than the column.
    pub fn for_sidebar(sidebar_width: f32) -> Self {
        let bar = sidebar_width - BAR_PADDING;
        if bar >= ALL_LABELS_FROM {
            return Self::All;
        }
        if bar >= ACTIVE_LABEL_FROM {
            return Self::ActiveOnly;
        }
        Self::Hidden
    }
}

#[cfg(test)]
#[path = "label_visibility_tests.rs"]
mod tests;
