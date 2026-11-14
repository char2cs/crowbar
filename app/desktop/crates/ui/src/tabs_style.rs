//! Looks of a [`crate::Tabs`] list.

/// Visual recipe of a tab list.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum TabsStyle {
    /// `TabsList`: 32px tabs on a muted track under a raised control-surface pill (sidebar mode switch).
    #[default]
    Pill,
    /// `SegmentedControl`: 30px segments, the indicator is the background colour (pane header switch).
    Segmented,
}
