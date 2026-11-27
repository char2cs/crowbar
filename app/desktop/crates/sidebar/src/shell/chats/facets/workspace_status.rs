#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WorkspaceStatus {
    Placeholder,
    Locked,
    New,
    PrConflicts,
    Deleted,
    PrClosed,
    PrOpen,
    PrMerged,
}
