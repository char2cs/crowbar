//! What the text area element needs from the field it paints.

use std::ops::Range;

use crate::selection_drag::SelectionDrag;
use gpui::{Bounds, EntityInputHandler, FocusHandle, Pixels, SharedString};

use crate::text_area_layout::TextAreaLayout;

pub(crate) trait TextAreaSurface: EntityInputHandler + SelectionDrag {
    fn content(&self) -> &str;
    fn hint(&self) -> &SharedString;
    fn marked_range(&self) -> Option<Range<usize>>;
    fn selected_range(&self) -> Range<usize>;
    fn cursor(&self) -> usize;
    fn scroll(&self) -> Pixels;
    fn revealed(&self) -> usize;
    fn editable(&self) -> bool;
    fn input_focus(&self) -> FocusHandle;
    fn record_layout(
        &mut self,
        layout: TextAreaLayout,
        bounds: Bounds<Pixels>,
        scroll: Pixels,
    );
}
