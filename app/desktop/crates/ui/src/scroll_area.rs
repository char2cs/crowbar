//! A scrolling region with the styled overlay scrollbar.

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    ScrollHandle, StatefulInteractiveElement, Styled, Window, div, hsla,
};

use crate::cubic_bezier::CubicBezier;
use crate::hover;
use crate::opaque_debug::opaque_debug;
use crate::pose::Pose;
use crate::scrollbar::Scrollbar;
use crate::timing;
use crate::tween::Tween;

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
    pub fn new(
        id: impl Into<ElementId>,
        handle: &ScrollHandle,
        content: impl IntoElement,
    ) -> Self {
        Self {
            id: id.into(),
            handle: handle.clone(),
            content: content.into_any_element(),
        }
    }
}

impl RenderOnce for ScrollArea {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let state = hover::track(&self.id, Pose::Off, window, cx);
        let shown = hover::position(
            &state,
            Tween::new(timing::MENU, CubicBezier::DEFAULT),
            window,
            cx,
        );
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
            .child(div().absolute().inset_0().child(Scrollbar::new(
                (self.id, "bar").into(),
                self.handle,
                color,
            )))
    }
}

opaque_debug!(ScrollArea);

#[cfg(test)]
mod tests {
    use gpui::{ParentElement, ScrollHandle, Styled, TestAppContext, div, px};

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn viewport_keeps_its_size_and_content_scrolls(cx: &mut TestAppContext) {
        let handle = ScrollHandle::new();
        let probe = handle.clone();
        let bounds = measure(cx, move |_, _| {
            let tall = div().h(px(600.0)).w(px(100.0));
            div()
                .h(px(100.0))
                .w(px(100.0))
                .child(ScrollArea::new("sa", &probe, tall))
        });
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (100.0, 100.0)
        );
        assert!(
            f32::from(handle.max_offset().height) > 400.0,
            "content is taller than the viewport"
        );
    }
}
