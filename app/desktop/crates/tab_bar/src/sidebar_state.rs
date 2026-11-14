/// Whether the sidebar is showing, and on which side it re-opens from.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum SidebarState {
    /// The sidebar is open and owns its own toggle.
    #[default]
    Open,
    /// Collapsed on the left: the strip shows the re-open toggle before the first tab.
    ClosedLeft,
    /// Collapsed on the right: the strip shows the mirrored toggle after the last tab.
    ClosedRight,
}
