/// Why the editor's documents could not do what was asked.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// The text to find could not be compiled.
    #[error("find: cannot search for {pattern:?}: {source}")]
    Pattern {
        /// The query as typed.
        pattern: String,
        /// What the regular expression parser reported.
        #[source]
        source: regex::Error,
    },
    /// A highlight query rewrite no longer finds the text it replaces.
    #[error("highlight query: {language}: anchor not found exactly once: {anchor:?}")]
    QueryAnchor {
        /// The language whose query changed.
        language: &'static str,
        /// The text the rewrite looked for.
        anchor: &'static str,
    },
}
