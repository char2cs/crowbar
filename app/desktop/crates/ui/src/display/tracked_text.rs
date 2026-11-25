//! Text set with letter spacing, as CSS `letter-spacing` sets develop's `tracking-*` text.

use gpui::{
    App, AvailableSpace, Bounds, Element, ElementId, FontId, GlobalElementId, InspectorElementId,
    LayoutId, Pixels, ShapedGlyph, ShapedLine, SharedString, Style, TextStyle, Window, point, size,
};

use crate::foundation::tokens::tracking;

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
        let rem = window.rem_size();
        let line = shape(self.text.clone(), &style, window);
        let natural = tracked_width(&line, spacing(self.em, &style, window));
        let height = style.line_height_in_pixels(rem);
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
        let spacing = spacing(self.em, &style, window);
        let line = shape(self.text.clone(), &style, window);
        let baseline = baseline(bounds, &line, &style, window);
        let fits = tracked_width(&line, spacing) <= bounds.size.width;
        if fits || style.text_overflow.is_none() {
            paint_glyphs((&line, spacing), glyph_count(&line), baseline, window);
            return;
        }
        let ellipsis = shape(ELLIPSIS.into(), &style, window);
        let room = bounds.size.width - tracked_width(&ellipsis, spacing);
        let kept = kept_glyphs(&line, spacing, room);
        paint_glyphs((&line, spacing), kept, baseline, window);
        let after = point(baseline.x + glyph_x(&line, spacing, kept), baseline.y);
        paint_glyphs((&ellipsis, spacing), glyph_count(&ellipsis), after, window);
    }
}

fn shape(
    text: SharedString,
    style: &TextStyle,
    window: &Window,
) -> ShapedLine {
    let font_size = style.font_size.to_pixels(window.rem_size());
    let run = style.to_run(text.len());
    window
        .text_system()
        .shape_line(text, font_size, &[run], None)
}

fn spacing(
    em: f32,
    style: &TextStyle,
    window: &Window,
) -> Pixels {
    tracking::at(em, style.font_size.to_pixels(window.rem_size()))
}

fn definite(space: AvailableSpace) -> Option<Pixels> {
    let AvailableSpace::Definite(limit) = space else {
        return None;
    };
    Some(limit)
}

fn glyphs(line: &ShapedLine) -> impl Iterator<Item = (FontId, &ShapedGlyph)> {
    line.runs
        .iter()
        .flat_map(|run| run.glyphs.iter().map(move |glyph| (run.font_id, glyph)))
}

fn glyph_count(line: &ShapedLine) -> usize {
    glyphs(line).count()
}

fn glyph_x(
    line: &ShapedLine,
    spacing: Pixels,
    index: usize,
) -> Pixels {
    let shaped = glyphs(line)
        .nth(index)
        .map_or(line.width, |(_, glyph)| glyph.position.x);
    shaped + spacing * glyphs_before(index)
}

fn glyphs_before(index: usize) -> f32 {
    u16::try_from(index).map_or(f32::from(u16::MAX), f32::from)
}

fn tracked_width(
    line: &ShapedLine,
    spacing: Pixels,
) -> Pixels {
    glyph_x(line, spacing, glyph_count(line))
}

fn kept_glyphs(
    line: &ShapedLine,
    spacing: Pixels,
    room: Pixels,
) -> usize {
    let count = glyph_count(line);
    (0..count)
        .take_while(|index| glyph_x(line, spacing, index + 1) - spacing <= room)
        .count()
}

fn baseline(
    bounds: Bounds<Pixels>,
    line: &ShapedLine,
    style: &TextStyle,
    window: &Window,
) -> gpui::Point<Pixels> {
    let line_height = style.line_height_in_pixels(window.rem_size());
    let padding = (line_height - line.ascent - line.descent) / 2.0;
    point(bounds.left(), bounds.top() + padding + line.ascent)
}

fn paint_glyphs(
    (line, spacing): (&ShapedLine, Pixels),
    count: usize,
    origin: gpui::Point<Pixels>,
    window: &mut Window,
) {
    let color = window.text_style().color;
    for (index, (font_id, glyph)) in glyphs(line).take(count).enumerate() {
        let x = origin.x + glyph.position.x + spacing * glyphs_before(index);
        let at = point(x, origin.y + glyph.position.y);
        log_failure(paint_glyph(
            at,
            (font_id, glyph),
            (line.font_size, color),
            window,
        ));
    }
}

fn paint_glyph(
    at: gpui::Point<Pixels>,
    (font_id, glyph): (FontId, &ShapedGlyph),
    (font_size, color): (Pixels, gpui::Hsla),
    window: &mut Window,
) -> gpui::Result<()> {
    if glyph.is_emoji {
        return window.paint_emoji(at, font_id, glyph.id, font_size);
    }
    window.paint_glyph(at, font_id, glyph.id, font_size, color)
}

fn log_failure(painted: gpui::Result<()>) {
    if let Err(error) = painted {
        tracing::warn!(%error, "painting a tracked glyph failed");
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement, Styled, TestAppContext, div, px};

    use super::*;
    use crate::test_support::probe::{measure, open};

    const SAMPLE: &str = "Crowbar";
    const SIZE: f32 = 20.0;
    const EM: f32 = 0.1;
    const NARROW: f32 = 30.0;

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
        let glyphs = glyphs_before(SAMPLE.chars().count());
        let expected = glyphs * EM * SIZE;
        assert!(
            (tracked - loose - expected).abs() < 0.01,
            "{tracked} - {loose} vs {expected}"
        );
    }

    #[gpui::test]
    fn a_narrow_box_keeps_the_glyphs_that_fit_before_the_ellipsis(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| div().text_size(px(SIZE)).child(SAMPLE));
        let (kept, count) = cx.update(|window, _| {
            let style = window.text_style();
            let spacing = spacing(EM, &style, window);
            let line = shape(SAMPLE.into(), &style, window);
            let ellipsis = tracked_width(&shape(ELLIPSIS.into(), &style, window), spacing);
            let kept = kept_glyphs(&line, spacing, px(NARROW) - ellipsis);
            (kept, glyph_count(&line))
        });
        assert_eq!(count, SAMPLE.chars().count(), "one glyph per letter");
        assert!(kept > 0 && kept < count, "{kept} of {count} fit");
    }
}
