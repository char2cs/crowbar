#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Outage {
    Unresponsive,
    Recovering { attempt: u32, reason: String },
    GaveUp { report: String },
    Stopped,
}
