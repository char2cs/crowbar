#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum TextInputEvent {
    Changed(String),
    Submitted,
    Cancelled,
}
