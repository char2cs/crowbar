use protocol::PaneId;

#[derive(Debug, Default)]
pub(crate) struct OwnedPane {
    pub(crate) pane: Option<PaneId>,
    pub(crate) generation: u64,
}
