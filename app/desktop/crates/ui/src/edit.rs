//! How an edit groups with the ones before it in the undo history.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Edit {
    Typing,
    Other,
}
