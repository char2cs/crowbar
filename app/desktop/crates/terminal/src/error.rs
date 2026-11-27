#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("search pattern is not valid: {reason}")]
    Pattern { reason: String },
}
