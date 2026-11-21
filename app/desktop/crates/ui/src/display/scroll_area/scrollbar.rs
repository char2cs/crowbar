//! The overlay scrollbar element.

use gpui::{
    App, BorderStyle, Bounds, Element, ElementId, GlobalElementId, Hsla, InspectorElementId,
    IntoElement, LayoutId, Pixels, ScrollHandle, Style, Window, point, px, quad, relative, size,
    transparent_black,
};

use crate::display::scroll_area::thumb::Thumb;
use crate::display::scroll_area::thumb_drag::ThumbDrag;
use crate::foundation::tokens::radius;

const WIDTH: f32 = 6.0;
const MARGIN: f32 = 4.0;

pub(crate) struct Scrollbar {
    handle: ScrollHandle,
    color: Hsla,
    id: ElementId,
}

impl Scrollbar {
    pub(crate) fn new(
        id: ElementId,
        handle: ScrollHandle,
        color: Hsla,
    ) -> Self {
        Self { handle, color, id }
    }
}

impl IntoElement for Scrollbar {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

impl Element for Scrollbar {
    type RequestLayoutState = ();
    type PrepaintState = Option<Bounds<Pixels>>;

    fn id(&self) -> Option<ElementId> {
        Some(self.id.clone())
    }

    fn source_location(&self) -> Option<&'static core::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        let mut style = Style::default();
        style.size.width = relative(1.0).into();
        style.size.height = relative(1.0).into();
        (window.request_layout(style, [], cx), ())
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        (): &mut Self::RequestLayoutState,
        _: &mut Window,
        _: &mut App,
    ) -> Self::PrepaintState {
        let scrolled = -f32::from(self.handle.offset().y);
        let max = f32::from(self.handle.max_offset().y);
        Thumb::locate(f32::from(bounds.size.height), max, scrolled).map(|thumb| {
            let origin = point(
                bounds.right() - px(MARGIN + WIDTH),
                bounds.top() + px(thumb.start),
            );
            Bounds::new(origin, size(px(WIDTH), px(thumb.length)))
        })
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        (): &mut Self::RequestLayoutState,
        prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        _: &mut App,
    ) {
        let Some(thumb) = *prepaint else {
            return;
        };
        window.paint_quad(quad(
            thumb,
            radius::FULL,
            self.color,
            0.0,
            transparent_black(),
            BorderStyle::default(),
        ));
        ThumbDrag::new(self.handle.clone(), f32::from(bounds.size.height), thumb).install(window);
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement, ScrollHandle, Styled, TestAppContext, div, hsla, px};

    use super::*;
    use crate::test_support::probe::measure;

    #[gpui::test]
    fn overlay_does_not_change_layout(cx: &mut TestAppContext) {
        let handle = ScrollHandle::new();
        let bounds = measure(cx, move |_, _| {
            div()
                .relative()
                .size(px(50.0))
                .child(div().absolute().inset_0().child(Scrollbar::new(
                    "bar".into(),
                    handle.clone(),
                    hsla(0.0, 0.0, 0.5, 0.5),
                )))
        });
        assert_eq!(f32::from(bounds.size.width), 50.0);
    }
}
