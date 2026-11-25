use protocol::{PaneId, PaneOrigin};

/// Who a web view acts for: the pane it shows and the origin that pane's code comes from.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Assignment {
    /// The pane shown in the web view.
    pub pane: PaneId,
    /// Where the pane's page comes from.
    pub origin: PaneOrigin,
}
