//! How a button between blocks is drawn.

/// The look of a [`super::ButtonSpec`] drawn between blocks.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ButtonFace {
    /// The small outline button.
    #[default]
    Button,
    /// A bordered label in body text, as develop draws its file-picker label.
    Label,
}
