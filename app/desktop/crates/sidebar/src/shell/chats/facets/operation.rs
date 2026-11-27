use crate::shell::chats::facets::operation_phase::OperationPhase;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Operation {
    pub label: String,
    pub phase: OperationPhase,
}
