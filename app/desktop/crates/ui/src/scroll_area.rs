//! A scrolling region with the styled overlay scrollbar.

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce, ScrollHandle,
    StatefulInteractiveElement, Styled, Window, div, hsla,
};

use crate::cubic_bezier::CubicBezier;
use crate::hover;
use crate::scrollbar::Scrollbar;
use crate::timing;

const THUMB_ALPHA: f32 = 0.42;
const THUMB_LIGHTNESS: f32 = 0.45;

/// Vertical scrolling container; the thumb fades in while the pointer is over the area.
#[derive(IntoElement)]
pub struct ScrollArea {
    id: ElementId,
    handle: ScrollHandle,
    content: AnyElement,
}

impl ScrollArea {
    /// A scroll area around `content`; keep the `handle` to read or set the offset.
    pub fn new(id: impl Into<ElementId>, handle: &ScrollHandle, content: impl IntoElement) -> Self {
        Self { id: id.into(), handle: handle.clone(), content: content.into_any_element() }
    }
}

impl RenderOnce for ScrollArea {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let state = hover::track(&self.id, "hover", false, window, cx);
        let shown = hover::position(&state, timing::MENU, CubicBezier::DEFAULT, window, cx);
        let color = hsla(0.0, 0.0, THUMB_LIGHTNESS, THUMB_ALPHA).opacity(shown);
        div()
            .id(self.id.clone())
            .relative()
            .size_full()
            .on_hover(hover::listener(&state))
            .child(
                div()
                    .id((self.id.clone(), "viewport"))
                    .size_full()
                    .overflow_y_scroll()
                    .track_scroll(&self.handle)
                    .child(self.content),
            )
            .child(div().absolute().inset_0().child(Scrollbar::new((self.id, "bar").into(), self.handle, color)))
    }
}

#[cfg(test)]
#[path = "scroll_area_tests.rs"]
mod tests;
