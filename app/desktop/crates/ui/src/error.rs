//! Errors raised while installing the design system.

/// Failure of [`crate::init`].
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The text system rejected an embedded font file.
    #[error("register embedded fonts: {0}")]
    Fonts(#[source] gpui::private::anyhow::Error),
}
