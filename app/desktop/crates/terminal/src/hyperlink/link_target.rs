//! What a terminal link points at.

/// The destination of a detected link.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum LinkTarget {
    /// A web address.
    Url(String),
    /// A file path as printed, with the line it names; resolve it with [`crate::resolve_file_path`].
    File {
        /// The path as printed.
        path: String,
        /// One-based line, when the output named one.
        line: Option<u32>,
    },
}
