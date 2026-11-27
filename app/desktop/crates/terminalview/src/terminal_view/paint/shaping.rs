mod cached_row;
pub(crate) mod line_cache;
mod row_input;
mod row_lines;

pub(crate) use line_cache::LineCache;
pub(crate) use row_input::RowInput;
pub(crate) use row_lines::RowLines;

use gpui::{Font, Pixels};

#[derive(Debug, Clone)]
pub(crate) struct Shaping {
    pub(crate) font: Font,
    pub(crate) font_size: Pixels,
    pub(crate) cell_width: Pixels,
}
