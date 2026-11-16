use protocol::PaneId;

#[derive(Debug, Clone, PartialEq)]
pub(crate) enum Remaining {
    Active(PaneId),
    Nothing,
}
