//! What colours the text, as develop's `semanticHighlighting`.

/// The sources of an editor view's colours.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum Highlighting {
    /// The grammar alone.
    Syntax,
    /// The grammar, then the language server's tokens.
    #[default]
    Semantic,
}
