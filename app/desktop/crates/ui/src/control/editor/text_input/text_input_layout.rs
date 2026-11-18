//! Alignment and the line a single-line field stores after painting.

use gpui::{Bounds, Pixels, ShapedLine};

pub(crate) trait TextInputLayout {
    fn centered(&self) -> bool;
    fn record_layout(
        &mut self,
        line: ShapedLine,
        bounds: Bounds<Pixels>,
    );
}
