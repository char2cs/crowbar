//! Whether long lines wrap, as Monaco's `wordWrap` option.

/// What an editor view does with a line longer than the pane is wide.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum WordWrap {
    /// The line runs on past the pane's edge and scrolls sideways.
    #[default]
    Off,
    /// The line wraps at the pane's width.
    On,
}
