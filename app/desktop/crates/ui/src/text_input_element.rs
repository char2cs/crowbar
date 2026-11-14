//! Paints the text, selection and caret of a [`TextInput`] and registers its input handler.

use std::time::Instant;

use gpui::{
    App, Bounds, Element, ElementId, ElementInputHandler, Entity, GlobalElementId, Hsla, InspectorElementId, IntoElement,
    LayoutId, PaintQuad, Pixels, ShapedLine, Style, TextRun, UnderlineStyle, Window, fill, point, px, relative, size,
};

use crate::keyframes::Keyframes;
use crate::motion::Motion;
use crate::palette::Palette;
use crate::text_input::TextInput;
use crate::timing;

const PLACEHOLDER_OPACITY: f32 = 0.72;
const CARET_WIDTH: f32 = 1.0;

/// The element that shapes and paints one line of a [`TextInput`].
pub(crate) struct TextInputElement {
    input: Entity<TextInput>,
    palette: &'static Palette,
}

impl TextInputElement {
    pub(crate) fn new(input: Entity<TextInput>, palette: &'static Palette) -> Self {
        Self { input, palette }
    }
}

/// Shaped line and the quads to paint around it.
pub(crate) struct Prepaint {
    line: ShapedLine,
    selection: Option<PaintQuad>,
    caret: Option<PaintQuad>,
}

impl IntoElement for TextInputElement {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

fn runs_for(len: usize, marked: Option<std::ops::Range<usize>>, base: &TextRun) -> Vec<TextRun> {
    let Some(marked) = marked else {
        return vec![TextRun { len, ..base.clone() }];
    };
    let underline = UnderlineStyle { color: Some(base.color), thickness: px(1.0), wavy: false };
    [
        TextRun { len: marked.start, ..base.clone() },
        TextRun { len: marked.end - marked.start, underline: Some(underline), ..base.clone() },
        TextRun { len: len - marked.end, ..base.clone() },
    ]
    .into_iter()
    .filter(|run| run.len > 0)
    .collect()
}

fn caret_color(color: Hsla, since: Instant, motion: Motion) -> Hsla {
    if motion.is_reduced() {
        return color;
    }
    let cycle = timing::CARET.as_secs_f32();
    let phase = (since.elapsed().as_secs_f32() % cycle) / cycle;
    color.opacity(Keyframes::CARET_BLINK.sample(phase))
}

impl Element for TextInputElement {
    type RequestLayoutState = ();
    type PrepaintState = Prepaint;

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
        _: &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        let input = self.input.read(cx);
        let empty = input.text().is_empty();
        let shown = if empty { input.placeholder().clone() } else { input.text().to_owned().into() };
        let style = window.text_style();
        let color = if empty { self.palette.muted_foreground().opacity(PLACEHOLDER_OPACITY) } else { style.color };
        let base = TextRun { len: shown.len(), font: style.font(), color, background_color: None, underline: None, strikethrough: None };
        let marked = input.buffer.marked().filter(|_| !empty);
        let line = window.text_system().shape_line(shown.clone(), style.font_size.to_pixels(window.rem_size()), &runs_for(shown.len(), marked, &base), None);
        let selection = input.buffer.selection();
        let (from, to) = (line.x_for_index(selection.start), line.x_for_index(selection.end));
        let height = bounds.size.height;
        if selection.is_empty() {
            let blink = caret_color(self.palette.foreground(), Instant::now(), Motion::of(cx));
            let caret = fill(Bounds::new(point(bounds.left() + from, bounds.top()), size(px(CARET_WIDTH), height)), blink);
            return Prepaint { line, selection: None, caret: Some(caret) };
        }
        let band = Bounds::new(point(bounds.left() + from, bounds.top()), size(to - from, height));
        Prepaint { line, selection: Some(fill(band, self.palette.editor_selection())), caret: None }
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        _: &mut Self::RequestLayoutState,
        prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        let focus = self.input.read(cx).focus_handle.clone();
        window.handle_input(&focus, ElementInputHandler::new(bounds, self.input.clone()), cx);
        if let Some(selection) = prepaint.selection.take() {
            window.paint_quad(selection);
        }
        let painted = prepaint.line.paint(bounds.origin, window.line_height(), window, cx);
        debug_assert!(painted.is_ok(), "shaped line failed to paint");
        let focused = focus.is_focused(window);
        if let Some(caret) = prepaint.caret.take().filter(|_| focused) {
            window.paint_quad(caret);
            window.request_animation_frame();
        }
        let line = prepaint.line.clone();
        self.input.update(cx, |input, _| {
            input.layout = Some(line);
            input.bounds = Some(bounds);
        });
    }
}

#[cfg(test)]
#[path = "text_input_element_tests.rs"]
mod tests;
