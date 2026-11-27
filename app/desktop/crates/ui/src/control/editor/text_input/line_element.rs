//! Paints the text, selection and caret of a text field and registers its input handler.

pub(super) mod caret_scroll;
pub(super) mod layout;
pub(super) mod line_surface;
mod scroll;

use std::ops::Range;

use gpui::{
    App, Bounds, Element, ElementId, ElementInputHandler, Entity, GlobalElementId, Hsla,
    InspectorElementId, LayoutId, Pixels, Point, SharedString, Style, TextRun, Window, fill, point,
    px, relative, size,
};

use crate::control::editor::field::Frame;
use crate::control::editor::field::capture_pointer;
use crate::control::editor::field::native_selection;
use crate::control::editor::field::{band_frame, blinks, caret_frame, runs_for, tend};
use crate::control::editor::text_input::line_element::caret_scroll::CARET_WIDTH;
use crate::control::editor::text_input::line_element::line_surface::LineSurface;
use crate::display::TrackedLine;
use crate::foundation::Palette;
use crate::foundation::Pose;
use crate::foundation::WindowActivity;

pub(crate) const PLACEHOLDER_OPACITY: f32 = 0.72;

pub(crate) struct LineElement<S: LineSurface> {
    input: Entity<S>,
    palette: &'static Palette,
}

impl<S: LineSurface> LineElement<S> {
    pub(crate) fn new(
        input: Entity<S>,
        palette: &'static Palette,
    ) -> Self {
        Self { input, palette }
    }

    fn aligned(
        &self,
        bounds: Bounds<Pixels>,
        line: &TrackedLine,
        cx: &App,
    ) -> Bounds<Pixels> {
        if !self.input.read(cx).centered() {
            return bounds;
        }
        let spare = (bounds.size.width - line.width()).max(px(0.0));
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
        (line, bounds): (TrackedLine, Bounds<Pixels>),
        selection: &Range<usize>,
    ) -> Frame {
        let (from, to) = (
            line.x_for_index(selection.start),
            line.x_for_index(selection.end),
        );
        let origin = point(bounds.left() + from, bounds.top());
        if selection.is_empty() {
            let caret = Bounds::new(origin, size(px(CARET_WIDTH), bounds.size.height));
            return caret_frame(line, fill(caret, self.palette.foreground()));
        }
        let band = Bounds::new(origin, size(to - from, bounds.size.height));
        band_frame(line, fill(band, native_selection()))
    }

    fn shape(
        &self,
        window: &mut Window,
        cx: &App,
    ) -> TrackedLine {
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
        let marked_free = marked.is_none();
        let runs = runs_for(shown.len(), marked, &base);
        let font_size = style.font_size.to_pixels(window.rem_size());
        let em = input.tracking() * f32::from(u8::from(marked_free));
        let line = window
            .text_system()
            .shape_line(shown, font_size, &runs, None);
        TrackedLine::of(line, crate::foundation::tracking::at(em, font_size), color)
    }
}

impl<S: LineSurface> Element for LineElement<S> {
    type RequestLayoutState = ();
    type PrepaintState = Frame;

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
        let visible = self.aligned(bounds, &line, cx);
        let (bounds, _) = scroll::place(self.input.read(cx), (visible, &line));
        let selection = self.input.read(cx).selected_range();
        self.frame((line, bounds), &selection)
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
        let visible = self.aligned(bounds, &prepaint.line, cx);
        let (bounds, scroll) = scroll::place(self.input.read(cx), (visible, &prepaint.line));
        capture_pointer(&self.input, window, cx);
        let focus = self.input.read(cx).input_focus();
        window.handle_input(
            &focus,
            ElementInputHandler::new(bounds, self.input.clone()),
            cx,
        );
        let blinks = window.with_content_mask(Some(scroll::clip(visible)), |window| {
            self.draw((prepaint, bounds.origin), window, cx)
        });
        let line = prepaint.line.clone();
        self.input.update(cx, |input, cx| {
            input.record_layout(line, (bounds, scroll));
            tend(input, blinks, cx);
        });
    }
}

impl<S: LineSurface> LineElement<S> {
    fn draw(
        &self,
        (prepaint, origin): (&mut Frame, Point<Pixels>),
        window: &mut Window,
        cx: &mut App,
    ) -> Pose {
        let focus = self.input.read(cx).input_focus();
        if let Some(selection) = prepaint.selection.take() {
            window.paint_quad(selection);
        }
        let painted = prepaint.line.paint_line(origin, window, cx);
        debug_assert!(painted.is_ok(), "shaped line failed to paint");
        let lit = focus.is_focused(window) && WindowActivity::of(window, cx);
        let caret = prepaint.caret.take().filter(|_| lit);
        let blinks = blinks(Pose::from(caret.is_some()), cx);
        let shown = self.input.read(cx).blink().is_shown() || blinks == Pose::Off;
        if let Some(caret) = caret.filter(|_| shown) {
            window.paint_quad(caret);
        }
        blinks
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::{Entity, TestAppContext, VisualTestContext};

    use crate::control::editor::field::Blinking as _;
    use crate::control::editor::test_support::field::{drawn_field, field};
    use crate::control::editor::text_input::TextInput;
    use crate::foundation::CaretBlink;
    use crate::foundation::Motion;
    use crate::foundation::caret_blink::GTK_DEFAULT_PHASE;

    const JUST_BEFORE: Duration = Duration::from_millis(1);
    const MANY_BLINKS: u32 = 10;

    fn shown(
        input: &Entity<TextInput>,
        cx: &mut VisualTestContext,
    ) -> bool {
        input.read_with(cx, |input, _| input.blink().is_shown())
    }

    #[gpui::test]
    fn a_focused_field_schedules_only_the_next_blink(cx: &mut TestAppContext) {
        let (input, draws, cx) = drawn_field(cx);
        let settled = draws.get();
        cx.executor()
            .advance_clock(GTK_DEFAULT_PHASE.saturating_sub(JUST_BEFORE));
        assert_eq!(draws.get(), settled, "no frame before the blink edge");
        assert!(shown(&input, cx), "the caret starts shown");
        cx.executor().advance_clock(JUST_BEFORE);
        assert_eq!(draws.get(), settled + 1, "one frame at the edge");
        assert!(!shown(&input, cx), "the edge hides the caret");
        cx.executor().advance_clock(GTK_DEFAULT_PHASE);
        assert_eq!(draws.get(), settled + 2, "one frame at the next edge");
        assert!(shown(&input, cx), "and the next edge shows it again");
    }

    #[gpui::test]
    fn an_inactive_window_requests_no_caret_frames(cx: &mut TestAppContext) {
        let (input, draws, cx) = drawn_field(cx);
        cx.deactivate_window();
        let settled = draws.get();
        cx.executor().advance_clock(GTK_DEFAULT_PHASE * MANY_BLINKS);
        assert_eq!(draws.get(), settled, "no frames while inactive");
        cx.update(|window, _| window.activate_window());
        cx.run_until_parked();
        assert!(shown(&input, cx), "the caret is back, shown, on return");
        let back = draws.get();
        cx.executor().advance_clock(GTK_DEFAULT_PHASE);
        assert_eq!(draws.get(), back + 1, "and blinks again");
    }

    #[gpui::test]
    fn the_caret_blinks_while_a_child_window_holds_the_keyboard(cx: &mut TestAppContext) {
        let (_, draws, cx) = drawn_field(cx);
        cx.deactivate_window();
        cx.update(|window, cx| {
            let id = window.window_handle().window_id();
            crate::WindowActivity::gained(id, cx);
        });
        cx.run_until_parked();
        let settled = draws.get();
        cx.executor().advance_clock(GTK_DEFAULT_PHASE);
        assert_eq!(
            draws.get(),
            settled + 1,
            "the window still holds the keyboard"
        );
    }

    #[gpui::test]
    fn typing_restarts_the_blink_phase(cx: &mut TestAppContext) {
        let (input, _, cx) = drawn_field(cx);
        cx.executor().advance_clock(GTK_DEFAULT_PHASE / 2);
        cx.simulate_input("a");
        cx.executor()
            .advance_clock(GTK_DEFAULT_PHASE.saturating_sub(JUST_BEFORE));
        assert!(shown(&input, cx), "the old edge passed without hiding it");
        cx.executor().advance_clock(JUST_BEFORE);
        assert!(!shown(&input, cx), "a full phase after the keystroke");
    }

    #[gpui::test]
    fn under_reduced_motion_the_caret_is_solid_and_requests_no_frames(cx: &mut TestAppContext) {
        let (input, draws, cx) = drawn_field(cx);
        cx.update(|_, cx| cx.set_global(Motion::reduced()));
        cx.update(|window, _| window.refresh());
        cx.run_until_parked();
        let settled = draws.get();
        cx.executor().advance_clock(GTK_DEFAULT_PHASE * MANY_BLINKS);
        assert_eq!(draws.get(), settled, "no blink frames");
        assert!(shown(&input, cx), "the caret stays shown");
    }

    #[gpui::test]
    fn a_steady_caret_setting_keeps_the_caret_shown_without_frames(cx: &mut TestAppContext) {
        let (input, draws, cx) = drawn_field(cx);
        cx.update(|_, cx| cx.set_global(CaretBlink::Steady));
        cx.update(|window, _| window.refresh());
        cx.run_until_parked();
        let settled = draws.get();
        cx.executor().advance_clock(GTK_DEFAULT_PHASE * MANY_BLINKS);
        assert_eq!(draws.get(), settled, "no blink frames");
        assert!(shown(&input, cx), "the caret stays shown");
    }

    #[gpui::test]
    fn painting_records_the_shaped_line_for_the_input_handler(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("abc");
        cx.run_until_parked();
        let recorded = input.read_with(cx, |input, _| input.layout.is_some());
        assert!(recorded, "the element stored its line");
    }
}
