use protocol::PaneId;

#[derive(Debug, Clone, Copy)]
pub(crate) struct SaveAsPanes<'a> {
    pub(crate) holder: &'a PaneId,
    pub(crate) untitled: &'a PaneId,
}
