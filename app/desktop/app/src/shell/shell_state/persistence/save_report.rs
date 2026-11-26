#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum SaveReport {
    Failed(String),
    Recovered,
}
