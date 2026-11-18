/// What happens to a pane's web view when the pane stops being visible.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PaneLifecycle {
    /// The page stays loaded in a hidden web view.
    KeepAlive,
    /// The web view goes back to the pool; the page is rebuilt from its state when shown.
    Parkable,
    /// The web view goes back to the pool and the pane's state is dropped.
    DestroyOnHide,
}
