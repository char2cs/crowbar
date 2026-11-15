#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Stage {
    /// Not in the active tab, or parked.
    Hidden,
    /// In the active tab but not the focused pane.
    Shown,
    /// The focused pane of the active tab, told to `activate`.
    Active,
}
