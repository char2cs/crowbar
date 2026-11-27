#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("find: cannot search for {pattern:?}: {source}")]
    Pattern {
        pattern: String,
        #[source]
        source: regex::Error,
    },
    #[error("highlight query: {language}: anchor not found exactly once: {anchor:?}")]
    QueryAnchor {
        language: &'static str,
        anchor: &'static str,
    },
}
