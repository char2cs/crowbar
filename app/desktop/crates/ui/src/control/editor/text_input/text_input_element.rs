//! Paints the text, selection and caret of a text field and registers its input handler.

use std::ops::Range;

use gpui::{
    App, Bounds, Element, ElementId, ElementInputHandler, Entity, GlobalElementId, Hsla,
    InspectorElementId, IntoElement, LayoutId, Pixels, ShapedLine, SharedString, Style, TextRun,
    Window, fill, point, px, relative, size,
};

use crate::control::editor::field::TextFrame;
use crate::control::editor::field::capture_pointer;
use crate::control::editor::field::{band_frame, caret_color, caret_frame, runs_for};
use crate::control::editor::text_input::text_surface::TextSurface;
use crate::foundation::Motion;
use crate::foundation::Palette;
use crate::foundation::motion::adapters::clock as system_clock;

pub(crate) const PLACEHOLDER_OPACITY: f32 = 0.72;
pub(crate) const CARET_WIDTH: f32 = 1.0;

pub(crate) struct TextInputElement<S: TextSurface> {
    input: Entity<S>,
    palette: &'static Palette,
}

impl<S: TextSurface> TextInputElement<S> {
    pub(crate) fn new(
        input: Entity<S>,
        palette: &'static Palette,
    ) -> Self {
        Self { input, palette }
    }

    fn aligned(
        &self,
        bounds: Bounds<Pixels>,
        line: &ShapedLine,
        cx: &App,
    ) -> Bounds<Pixels> {
        if !self.input.read(cx).centered() {
            return bounds;
        }
        let spare = (bounds.size.width - line.width).max(px(0.0));
        Bounds::new(
            point(bounds.left() + spare / 2.0, bounds.top()),
            size(bounds.size.width - spare, bounds.size.height),
        )
    }

    fn visible(
        &self,
        input: &S,
        text_color: Hsla,
    ) -> (SharedString, Hsla) {
        if input.content().is_empty() {
            let hint = self.palette.muted_foreground();
            return (input.hint().clone(), hint.opacity(PLACEHOLDER_OPACITY));
        }
        (input.content().to_owned().into(), text_color)
    }

    fn frame(
        &self,
        (line, bounds): (ShapedLine, Bounds<Pixels>),
        selection: &Range<usize>,
        motion: Motion,
    ) -> TextFrame {
        let (from, to) = (
            line.x_for_index(selection.start),
            line.x_for_index(selection.end),
        );
        let origin = point(bounds.left() + from, bounds.top());
        if selection.is_empty() {
            let blink = caret_color(self.palette.foreground(), system_clock::now(), motion);
            let caret = Bounds::new(origin, size(px(CARET_WIDTH), bounds.size.height));
            return caret_frame(line, fill(caret, blink));
        }
        let band = Bounds::new(origin, size(to - from, bounds.size.height));
        band_frame(line, fill(band, self.palette.editor_selection()))
    }

    fn shape(
        &self,
        window: &mut Window,
        cx: &App,
    ) -> ShapedLine {
        let input = self.input.read(cx);
        let empty = input.content().is_empty();
        let style = window.text_style();
        let (shown, color) = self.visible(input, style.color);
        let base = TextRun {
            len: shown.len(),
            font: style.font(),
            color,
            background_color: None,
            underline: None,
            strikethrough: None,
        };
        let marked = input.marked_range().filter(|_| !empty);
        let runs = runs_for(shown.len(), marked, &base);
        window.text_system().shape_line(
            shown,
            style.font_size.to_pixels(window.rem_size()),
            &runs,
            None,
        )
    }
}

impl<S: TextSurface> IntoElement for TextInputElement<S> {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

impl<S: TextSurface> Element for TextInputElement<S> {
    type RequestLayoutState = ();
    type PrepaintState = TextFrame;

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
        cx: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        let mut style = Style::default();
        style.size.width = relative(1.0).into();
        style.size.height = window.line_height().into();
        (window.request_layout(style, [], cx), ())
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        (): &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        let line = self.shape(window, cx);
        let bounds = self.aligned(bounds, &line, cx);
        let selection = self.input.read(cx).selected_range();
        self.frame((line, bounds), &selection, Motion::of(cx))
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        (): &mut Self::RequestLayoutState,
        prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        let bounds = self.aligned(bounds, &prepaint.line, cx);
        capture_pointer(&self.input, window, cx);
        let focus = self.input.read(cx).input_focus();
        window.handle_input(
            &focus,
            ElementInputHandler::new(bounds, self.input.clone()),
            cx,
        );
        if let Some(selection) = prepaint.selection.take() {
            window.paint_quad(selection);
        }
        let painted = prepaint
            .line
            .paint(bounds.origin, window.line_height(), window, cx);
        debug_assert!(painted.is_ok(), "shaped line failed to paint");
        let focused = focus.is_focused(window);
        if let Some(caret) = prepaint.caret.take().filter(|_| focused) {
            window.paint_quad(caret);
            window.request_animation_frame();
        }
        let line = prepaint.line.clone();
        self.input
            .update(cx, |input, _| input.record_layout(line, bounds));
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::control::editor::test_support::field::field;

    #[gpui::test]
    fn painting_records_the_shaped_line_for_the_input_handler(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.run_until_parked();
        let recorded = input.read_with(cx, |input, _| input.layout.is_some());
        assert!(recorded, "the element stored its line");
    }
}
