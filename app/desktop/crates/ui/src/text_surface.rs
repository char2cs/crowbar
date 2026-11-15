//! What the text element needs from the field it paints.

use std::ops::Range;

use gpui::{Bounds, EntityInputHandler, FocusHandle, Pixels, ShapedLine, SharedString};

pub(crate) trait TextSurface: EntityInputHandler {
    fn centered(&self) -> bool;
    fn content(&self) -> &str;
    fn hint(&self) -> &SharedString;
    fn marked_range(&self) -> Option<Range<usize>>;
    fn selected_range(&self) -> Range<usize>;
    fn input_focus(&self) -> FocusHandle;
    fn record_layout(
        &mut self,
        line: ShapedLine,
        bounds: Bounds<Pixels>,
    );
}
