use serde::{Deserialize, Serialize};

/// A lifecycle event the shell sends to a pane.
///
/// Hiding a pane's web view behind a menu or popover is not an event: it is a
/// shell-internal visibility change that no pane observes.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Lifecycle {
    /// The pane's page was created; it should render from the carried state.
    Mount,
    /// The pane became the visible, focused pane of its tab.
    Activate,
    /// The pane stopped being the active pane but stays loaded.
    Deactivate,
    /// The web view is about to be destroyed; the pane should flush its state.
    Park,
    /// The pane instance is removed for good; its state is dropped.
    Destroy,
}
