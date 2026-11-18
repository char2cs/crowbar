//! A row and the lines shaped from it.

use gpui::ShapedLine;
use terminal::Row;

pub(crate) struct CachedRow {
    pub(crate) row: Row,
    pub(crate) lines: Vec<ShapedLine>,
}
