#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ImportEvent {
    Browse,
    Add { folder: String, name: String },
    Close,
}
