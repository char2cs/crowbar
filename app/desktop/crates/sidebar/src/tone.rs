//! The colour of a menu entry.

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum Tone {
    #[default]
    Normal,
    Destructive,
}
