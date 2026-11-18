//! A path found in text.

use std::ops::Range;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PathMatch {
    pub(crate) span: Range<usize>,
    pub(crate) path_end: usize,
    pub(crate) line: Option<u32>,
}
