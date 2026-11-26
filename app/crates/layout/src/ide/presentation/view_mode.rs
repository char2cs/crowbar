/// How a group arranges its chat against its editor, as develop's `PaneViewPresentation`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ViewMode {
    /// Chat and editor side by side.
    SideBySide,
    /// Chat above the editor.
    Stacked,
    /// One of them at a time, picked from the tab bar.
    Tabs,
}
