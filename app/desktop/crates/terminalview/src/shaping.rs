//! What text shaping needs to know.

use gpui::{Font, Pixels};

#[derive(Debug, Clone)]
pub(crate) struct Shaping {
    pub(crate) font: Font,
    pub(crate) font_size: Pixels,
    pub(crate) cell_width: Pixels,
}
