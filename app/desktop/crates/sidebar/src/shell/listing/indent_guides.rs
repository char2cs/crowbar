//! Whether nested tree rows draw a rule per ancestor level.

/// The `renderIndentGuides` setting.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum IndentGuides {
    /// One rule per ancestor level beside nested rows.
    #[default]
    Drawn,
    /// No rules.
    Hidden,
}
