use protocol::{PaneId, PaneOrigin};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Assignment {
    pub(crate) pane: PaneId,
    pub(crate) origin: PaneOrigin,
}
