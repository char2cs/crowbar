use protocol::{PaneId, PaneOrigin};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Assignment {
    pub pane: PaneId,
    pub origin: PaneOrigin,
}
