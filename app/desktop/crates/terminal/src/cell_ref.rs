//! A grid cell addressed by absolute line.

use alacritty_terminal::index::Line;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct CellRef {
    pub(crate) line: Line,
    pub(crate) column: usize,
    pub(crate) width: usize,
}
