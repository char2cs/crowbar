#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum RecordCommand {
    SetFlag { flag: String, on: bool },
    Move { to: usize },
    Run { action: String },
    Refresh,
}
