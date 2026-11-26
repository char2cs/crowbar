#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum PanePhase {
    /// No web view; the pane is parked or was never shown.
    Parked,
    /// A web view is loading the page; the pane has not registered yet.
    Loading,
    /// The page registered and was mounted.
    Mounted,
    /// The pane cannot run; its web view stays hidden.
    Failed,
}
