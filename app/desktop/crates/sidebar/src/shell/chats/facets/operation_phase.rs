#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OperationPhase {
    Running,
    Failed { reason: String },
}
