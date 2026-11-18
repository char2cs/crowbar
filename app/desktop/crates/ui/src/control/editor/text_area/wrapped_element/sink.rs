//! Takes the layout a text area computed while painting.

use gpui::{Bounds, Pixels};

use crate::control::editor::text_area::layout::Layout;

pub(crate) trait Sink {
    fn record_layout(
        &mut self,
        layout: Layout,
        bounds: Bounds<Pixels>,
        scroll: Pixels,
    );
}
