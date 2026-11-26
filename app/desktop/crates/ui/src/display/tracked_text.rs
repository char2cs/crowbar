//! Text set with letter spacing, as CSS `letter-spacing` sets develop's `tracking-*` text.

mod tracked_line;

use gpui::{
    App, AvailableSpace, Bounds, Element, ElementId, GlobalElementId, InspectorElementId, LayoutId,
    Pixels, SharedString, Style, TextStyle, Window, point, size,
};

use tracked_line::TrackedLine;

const ELLIPSIS: &str = "\u{2026}";

/// One line of text with `em` of its font size added after every glyph, styled by the text
/// style around it. Where that style truncates (`.truncate()`), it ends in an ellipsis that
/// fits the width it is given.
#[derive(Debug)]
pub struct TrackedText {
    text: SharedString,
    em: f32,
}

impl TrackedText {
    /// `text` spaced by `em` (see [`crate::tracking`]).
    pub fn new(
        text: impl Into<SharedString>,
        em: f32,
    ) -> Self {
        Self {
            text: text.into(),
            em,
        }
    }
}

impl Element for TrackedText {
    type RequestLayoutState = ();
    type PrepaintState = ();

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
    ) -> (LayoutId, ()) {
        let style = window.text_style();
        let natural = TrackedLine::shape(self.text.clone(), &style, self.em, window).width();
        let height = style.line_height_in_pixels(window.rem_size());
        let truncates = style.text_overflow.is_some();
        let measure = move |known: gpui::Size<Option<Pixels>>,
                            available: gpui::Size<AvailableSpace>,
                            _: &mut Window,
                            _: &mut App| {
            let width = known.width.unwrap_or(natural);
            let limit = definite(available.width).filter(|_| truncates);
            size(limit.map_or(width, |limit| width.min(limit)), height)
        };
        (
            window.request_measured_layout(Style::default(), measure),
            (),
        )
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        _: Bounds<Pixels>,
        (): &mut (),
        _: &mut Window,
        _: &mut App,
    ) {
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        (): &mut (),
        (): &mut (),
        window: &mut Window,
        _: &mut App,
    ) {
        let style = window.text_style();
        let line = TrackedLine::shape(self.text.clone(), &style, self.em, window);
        let baseline = baseline(bounds, &line, &style, window);
        if line.width() <= bounds.size.width || style.text_overflow.is_none() {
            line.paint(line.count(), baseline, window);
            return;
        }
        let ellipsis = TrackedLine::shape(ELLIPSIS.into(), &style, self.em, window);
        let kept = line.kept_within(bounds.size.width - ellipsis.width());
        line.paint(kept, baseline, window);
        let after = point(baseline.x + line.x_at(kept), baseline.y);
        ellipsis.paint(ellipsis.count(), after, window);
    }
}

fn definite(space: AvailableSpace) -> Option<Pixels> {
    let AvailableSpace::Definite(limit) = space else {
        return None;
    };
    Some(limit)
}

fn baseline(
    bounds: Bounds<Pixels>,
    line: &TrackedLine,
    style: &TextStyle,
    window: &Window,
) -> gpui::Point<Pixels> {
    let line_height = style.line_height_in_pixels(window.rem_size());
    let (ascent, descent) = line.extent();
    let padding = (line_height - ascent - descent) / 2.0;
    point(bounds.left(), bounds.top() + padding + ascent)
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement, Styled, TestAppContext, div, px};

    use super::*;
    use crate::test_support::probe::measure;

    const SAMPLE: &str = "Crowbar";
    const SIZE: f32 = 20.0;
    const EM: f32 = 0.1;

    fn width(
        cx: &mut TestAppContext,
        em: f32,
    ) -> f32 {
        let bounds = measure(cx, move |_, _| {
            div()
                .text_size(px(SIZE))
                .child(TrackedText::new(SAMPLE, em))
        });
        f32::from(bounds.size.width)
    }

    #[gpui::test]
    fn spacing_adds_its_space_after_every_glyph(cx: &mut TestAppContext) {
        let loose = width(cx, 0.0);
        let tracked = width(cx, EM);
        let glyphs = f32::from(u8::try_from(SAMPLE.chars().count()).unwrap_or(u8::MAX));
        let expected = glyphs * EM * SIZE;
        assert!(
            (tracked - loose - expected).abs() < 0.01,
            "{tracked} - {loose} vs {expected}"
        );
    }
}
