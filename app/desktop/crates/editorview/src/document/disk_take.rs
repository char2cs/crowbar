#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DiskTake {
    Kept,
    Taken,
    Conflicted,
}
