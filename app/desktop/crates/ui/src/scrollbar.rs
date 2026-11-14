//! The overlay scrollbar element.

use gpui::{
    App, BorderStyle, Bounds, Element, ElementId, GlobalElementId, Hsla, InspectorElementId, IntoElement, LayoutId,
    MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent, PaintQuad, Pixels, ScrollHandle, Style, Window, point,
    px, quad, relative, size, transparent_black,
};

use crate::radius;
use crate::scroll_thumb::Thumb;

const WIDTH: f32 = 6.0;
const MARGIN: f32 = 4.0;

/// Paints a vertical thumb over a scrolling region and lets the pointer drag it.
pub(crate) struct Scrollbar {
    handle: ScrollHandle,
    color: Hsla,
    id: ElementId,
}

impl Scrollbar {
    pub(crate) fn new(id: ElementId, handle: ScrollHandle, color: Hsla) -> Self {
        Self { handle, color, id }
    }
}

/// The thumb quad of this frame, in window coordinates.
pub(crate) struct Prepaint {
    thumb: Option<Bounds<Pixels>>,
}

impl IntoElement for Scrollbar {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

impl Element for Scrollbar {
    type RequestLayoutState = ();
    type PrepaintState = Prepaint;

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
        _: &mut Self::RequestLayoutState,
        _: &mut Window,
        _: &mut App,
    ) -> Self::PrepaintState {
        let scrolled = -f32::from(self.handle.offset().y);
        let max = f32::from(self.handle.max_offset().height);
        let thumb = Thumb::locate(f32::from(bounds.size.height), max, scrolled).map(|thumb| {
            let origin = point(bounds.right() - px(MARGIN + WIDTH), bounds.top() + px(thumb.start));
            Bounds::new(origin, size(px(WIDTH), px(thumb.length)))
        });
        Prepaint { thumb }
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        _: &mut Self::RequestLayoutState,
        prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        _: &mut App,
    ) {
        let Some(thumb) = prepaint.thumb else {
            return;
        };
        let paint: PaintQuad = quad(thumb, radius::FULL, self.color, 0.0, transparent_black(), BorderStyle::default());
        window.paint_quad(paint);
        let handle = self.handle.clone();
        let view = f32::from(bounds.size.height);
        let grab = std::rc::Rc::new(std::cell::Cell::new(None::<(f32, f32)>));
        let on_down = std::rc::Rc::clone(&grab);
        let down_handle = handle.clone();
        window.on_mouse_event(move |event: &MouseDownEvent, _, window, _| {
            if event.button == MouseButton::Left && thumb.contains(&event.position) {
                on_down.set(Some((f32::from(event.position.y), -f32::from(down_handle.offset().y))));
                window.prevent_default();
            }
        });
        let on_move = std::rc::Rc::clone(&grab);
        window.on_mouse_event(move |event: &MouseMoveEvent, _, window, _| {
            let Some((start_y, start_offset)) = on_move.get() else {
                return;
            };
            let max = f32::from(handle.max_offset().height);
            let delta = f32::from(event.position.y) - start_y;
            let target = Thumb::scrolled_after(view, max, start_offset, delta);
            handle.set_offset(point(handle.offset().x, px(-target)));
            window.refresh();
        });
        window.on_mouse_event(move |_: &MouseUpEvent, _, _, _| grab.set(None));
    }
}

#[cfg(test)]
#[path = "scrollbar_tests.rs"]
mod tests;
