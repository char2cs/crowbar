#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Report {
    Bound(String),
    Gone,
    Failed(String),
    StopFailed(String),
}
