//! How a row draws its label.

/// The type a row's label takes.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum LabelStyle {
    /// A form label: foreground ink at a fixed size.
    #[default]
    Label,
    /// A muted caption in the scaled body size, for a row that names a control without a form label.
    Caption,
}
