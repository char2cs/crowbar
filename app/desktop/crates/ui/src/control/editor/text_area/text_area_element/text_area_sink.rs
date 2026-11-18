//! Takes the layout a text area computed while painting.

use gpui::{Bounds, Pixels};

use crate::control::editor::text_area::text_area_layout::TextAreaLayout;

pub(crate) trait TextAreaSink {
    fn record_layout(
        &mut self,
        layout: TextAreaLayout,
        bounds: Bounds<Pixels>,
        scroll: Pixels,
    );
}
