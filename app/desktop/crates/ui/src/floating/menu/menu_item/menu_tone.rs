//! Colour treatment of a menu row.

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum MenuTone {
    #[default]
    Normal,
    Destructive,
}
