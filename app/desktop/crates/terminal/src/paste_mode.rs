//! How pasted text is wrapped.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum PasteMode {
    Plain,
    Bracketed,
}
