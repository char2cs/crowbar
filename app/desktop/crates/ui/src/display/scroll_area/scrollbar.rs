use gpui::{
    App, BorderStyle, Bounds, Corners, Element, ElementId, GlobalElementId, Hsla,
    InspectorElementId, LayoutId, PaintQuad, Pixels, ScrollHandle, Style, Window, point, px, quad,
    relative, size, transparent_black,
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
        window.paint_quad(thumb_quad(thumb, self.color));
        ThumbDrag::new(self.handle.clone(), f32::from(bounds.size.height), thumb).install(window);
    }
}

fn thumb_quad(
    thumb: Bounds<Pixels>,
    color: Hsla,
) -> PaintQuad {
    let ends = Corners::all(radius::FULL).clamp_radii_for_quad_size(thumb.size);
    quad(
        thumb,
        ends,
        color,
        0.0,
        transparent_black(),
        BorderStyle::default(),
    )
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

    #[test]
    fn the_thumb_rounds_its_ends_within_its_own_width() {
        let thumb = Bounds::new(point(px(0.0), px(0.0)), size(px(WIDTH), px(40.0)));
        let painted = thumb_quad(thumb, hsla(0.0, 0.0, 0.5, 0.5));
        let widest = [
            painted.corner_radii.top_left,
            painted.corner_radii.top_right,
            painted.corner_radii.bottom_right,
            painted.corner_radii.bottom_left,
        ]
        .into_iter()
        .fold(px(0.0), Pixels::max);
        assert_eq!(
            widest,
            px(WIDTH / 2.0),
            "an unclamped radius past half the width leaves the quad unpainted"
        );
    }
}
