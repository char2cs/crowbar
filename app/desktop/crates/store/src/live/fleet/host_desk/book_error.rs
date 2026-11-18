//! Why the transport could not probe, pair or unpair a host.

/// The transport's own failure, kept as its source; its text is what the pairing dialog turns
/// into words.
#[derive(Debug, thiserror::Error)]
#[error("{source}")]
pub struct BookError {
    #[source]
    source: Box<dyn std::error::Error + Send + Sync>,
}

impl BookError {
    /// Wraps the transport's failure.
    pub fn new(source: impl std::error::Error + Send + Sync + 'static) -> Self {
        Self {
            source: Box::new(source),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_text_is_the_causes_and_the_cause_stays_reachable() {
        let error = BookError::new(std::io::Error::other("cannot reach box:3737"));
        assert_eq!(error.to_string(), "cannot reach box:3737");
        let source = std::error::Error::source(&error).map(ToString::to_string);
        assert_eq!(source.as_deref(), Some("cannot reach box:3737"));
    }
}
