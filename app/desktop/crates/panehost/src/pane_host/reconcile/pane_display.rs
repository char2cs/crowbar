use protocol::PaneTypeId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PaneDisplay {
    Web,
    Native,
    Empty,
    Placeholder { type_id: PaneTypeId },
    Error { message: String },
}
