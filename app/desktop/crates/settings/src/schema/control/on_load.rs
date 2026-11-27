//! What develop's settings store does to a stored number when it loads it.

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) enum OnLoad {
    Kept,
    Normalized,
}
