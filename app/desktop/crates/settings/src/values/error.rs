use crate::values::key::Key;

#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error("settings schema: {what} `{name}` is declared twice")]
    Duplicate { what: &'static str, name: String },
    #[error("setting {key} was not saved: {reason}")]
    Rejected { key: Key, reason: String },
    #[error("settings backend is unavailable: {0}")]
    Unavailable(String),
}
