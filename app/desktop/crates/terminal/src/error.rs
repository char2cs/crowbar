//! Errors of the terminal crate.

/// Why a terminal operation failed.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// A search pattern did not compile.
    #[error("search pattern is not valid: {reason}")]
    Pattern {
        /// What the regex engine said.
        reason: String,
    },
}
