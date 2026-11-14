//! Which way a cursor moves through a list or a line of text.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Direction {
    Forward,
    Backward,
}
