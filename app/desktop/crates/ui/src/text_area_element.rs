//! Paints the wrapped text, selection and caret of a text area and registers its input handler.

use gpui::{
    App, AvailableSpace, Bounds, ContentMask, Element, ElementId, ElementInputHandler, Entity,
    GlobalElementId, InspectorElementId, IntoElement, LayoutId, Pixels, Size, Style, Window, fill,
    point, px, relative, size,
};

use crate::adapters::system_clock;
use crate::motion::Motion;
use crate::palette::Palette;
use crate::pose::Pose;
use crate::selection_drag::capture_pointer;
use crate::text_area_frame::TextAreaFrame;
use crate::text_area_layout::TextAreaLayout;
use crate::text_area_scroll::{clamp, reveal};
use crate::text_area_shape::TextAreaShape;
use crate::text_area_surface::TextAreaSurface;
use crate::text_input_element::{CARET_WIDTH, caret_color};
use crate::viewport::Viewport;

pub(crate) struct TextAreaElement<S: TextAreaSurface> {
    area: Entity<S>,
    palette: &'static Palette,
    limit: Option<Pixels>,
}

impl<S: TextAreaSurface> TextAreaElement<S> {
    pub(crate) fn new(
        area: Entity<S>,
        palette: &'static Palette,
        limit: Option<Pixels>,
    ) -> Self {
        Self {
            area,
            palette,
            limit,
        }
    }

    fn scrolled(
        area: &S,
        layout: &TextAreaLayout,
        height: Pixels,
    ) -> Pixels {
        let view = Viewport {
            height,
            content: layout.height(),
        };
        let spot = layout.point_for_index(area.cursor());
        let wanted = (spot.y, spot.y + layout.line_height());
        let moved = area.cursor() != area.revealed();
        moved.then_some(wanted).map_or_else(
            || clamp(area.scroll(), &view),
            |wanted| reveal(area.scroll(), &view, wanted),
        )
    }

    fn frame(
        &self,
        layout: TextAreaLayout,
        bounds: Bounds<Pixels>,
        cx: &App,
    ) -> TextAreaFrame {
        let area = self.area.read(cx);
        let scroll = Self::scrolled(area, &layout, bounds.size.height);
        let origin = point(bounds.left(), bounds.top() - scroll);
        let selection = area.selected_range();
        let bands = layout
            .selection_bounds(&selection)
            .into_iter()
            .map(|band| {
                fill(
                    Bounds::new(origin + band.origin, band.size),
                    self.palette.editor_selection(),
                )
            })
            .collect();
        let blink = caret_color(
            self.palette.foreground(),
            system_clock::now(),
            Motion::of(cx),
        );
        let spot = layout.point_for_index(area.cursor());
        let caret = Bounds::new(origin + spot, size(px(CARET_WIDTH), layout.line_height()));
        TextAreaFrame {
            caret: selection.is_empty().then(|| fill(caret, blink)),
            layout,
            origin,
            scroll,
            selection: bands,
        }
    }
}

fn paint_frame(
    frame: &mut TextAreaFrame,
    caret: Pose,
    window: &mut Window,
    cx: &mut App,
) {
    for band in frame.selection.drain(..) {
        window.paint_quad(band);
    }
    frame.layout.paint(frame.origin, window, cx);
    let blinking = frame.caret.take().filter(|_| caret == Pose::On);
    if let Some(quad) = blinking {
        window.paint_quad(quad);
        window.request_animation_frame();
    }
}

fn wrap_width(
    known: Size<Option<Pixels>>,
    available: Size<AvailableSpace>,
) -> Option<Pixels> {
    known.width.or(match available.width {
        AvailableSpace::Definite(width) => Some(width),
        AvailableSpace::MinContent | AvailableSpace::MaxContent => None,
    })
}

impl<S: TextAreaSurface> IntoElement for TextAreaElement<S> {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

impl<S: TextAreaSurface> Element for TextAreaElement<S> {
    type RequestLayoutState = TextAreaShape<S>;
    type PrepaintState = TextAreaFrame;

    fn id(&self) -> Option<ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static core::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        window: &mut Window,
        _: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        let shape =
            TextAreaShape::capture(self.area.clone(), self.palette.muted_foreground(), window);
        let measure = shape.clone();
        let limit = self.limit;
        let mut style = Style::default();
        style.size.width = relative(1.0).into();
        let layout = window.request_measured_layout(style, move |known, available, window, cx| {
            let wrap = wrap_width(known, available);
            let laid = measure.layout(wrap, window, cx);
            let height = limit.map_or(laid.height(), |most| laid.height().min(most));
            size(wrap.unwrap_or_else(|| laid.width()), height)
        });
        (layout, shape)
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        shape: &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        let layout = shape.layout(Some(bounds.size.width), window, cx);
        self.frame(layout, bounds, cx)
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        _: &mut Self::RequestLayoutState,
        frame: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        capture_pointer(&self.area, window, cx);
        let focus = self.area.read(cx).input_focus();
        let editable = self.area.read(cx).editable();
        if editable {
            let handler = ElementInputHandler::new(bounds, self.area.clone());
            window.handle_input(&focus, handler, cx);
        }
        let caret = Pose::from(editable && focus.is_focused(window));
        window.with_content_mask(Some(ContentMask { bounds }), |window| {
            paint_frame(frame, caret, window, cx);
        });
        let layout = frame.layout.clone();
        let scroll = frame.scroll;
        self.area
            .update(cx, |area, _| area.record_layout(layout, bounds, scroll));
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AvailableSpace, TestAppContext, px, size};

    use super::*;
    use crate::test_support::{area, area_text, area_with};
    use crate::text_area::TextArea;

    fn grown_height(
        entity: &Entity<TextArea>,
        cx: &mut gpui::VisualTestContext,
    ) -> f32 {
        entity.read_with(cx, |area, _| {
            area.bounds
                .map_or(0.0, |bounds| f32::from(bounds.size.height))
        })
    }

    #[test]
    fn wrapping_uses_the_known_width_then_the_definite_available_width() {
        type Case = (Option<f32>, AvailableSpace, Option<f32>);
        const CASES: &[Case] = &[
            (
                Some(120.0),
                AvailableSpace::Definite(px(300.0)),
                Some(120.0),
            ),
            (None, AvailableSpace::Definite(px(300.0)), Some(300.0)),
            (None, AvailableSpace::MaxContent, None),
            (None, AvailableSpace::MinContent, None),
        ];
        for &(known, available, want) in CASES {
            let known = size(known.map(px), None);
            let available = size(available, AvailableSpace::MinContent);
            assert_eq!(
                wrap_width(known, available),
                want.map(px),
                "{known:?} {available:?}"
            );
        }
    }

    #[gpui::test]
    fn the_text_height_grows_with_its_lines(cx: &mut TestAppContext) {
        let (entity, cx) = area(cx);
        let empty = grown_height(&entity, cx);
        entity.update(cx, |area, cx| area.set_text("1\n2\n3\n4\n5\n6", cx));
        cx.simulate_keystrokes("ctrl-end");
        let six_rows = grown_height(&entity, cx);
        assert_eq!((empty, six_rows), (20.0, 120.0));
    }

    #[gpui::test]
    fn past_its_maximum_height_the_field_scrolls_to_follow_the_caret(cx: &mut TestAppContext) {
        let (entity, cx) = area_with(cx, |area| area.max_height(px(72.0)));
        entity.update(cx, |area, cx| area.set_text("1\n2\n3\n4\n5\n6\n7\n8", cx));
        cx.simulate_keystrokes("ctrl-end");
        let (height, scroll) = entity.read_with(cx, |area, _| (grown_height_of(area), area.scroll));
        assert_eq!(height, 60.0);
        assert_eq!(scroll, px(100.0));
        cx.simulate_keystrokes("ctrl-home");
        let back = entity.read_with(cx, |area, _| area.scroll);
        assert_eq!(back, px(0.0));
        assert_eq!(area_text(&entity, cx).lines().count(), 8);
    }

    fn grown_height_of(area: &TextArea) -> f32 {
        area.bounds
            .map_or(0.0, |bounds| f32::from(bounds.size.height))
    }
}
