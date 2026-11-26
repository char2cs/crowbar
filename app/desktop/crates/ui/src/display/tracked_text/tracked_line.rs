//! A shaped line with letter spacing: where each glyph sits once the spacing is added after every
//! glyph before it, and how it paints.

use gpui::{FontId, Hsla, Pixels, Point, ShapedGlyph, ShapedLine, SharedString, TextStyle, Window};

use crate::foundation::tokens::tracking;

pub(super) struct TrackedLine {
    line: ShapedLine,
    spacing: Pixels,
    color: Hsla,
}

impl TrackedLine {
    pub(super) fn shape(
        text: SharedString,
        style: &TextStyle,
        em: f32,
        window: &Window,
    ) -> Self {
        let font_size = style.font_size.to_pixels(window.rem_size());
        let run = style.to_run(text.len());
        let line = window
            .text_system()
            .shape_line(text, font_size, &[run], None);
        Self {
            line,
            spacing: tracking::at(em, font_size),
            color: style.color,
        }
    }

    pub(super) fn extent(&self) -> (Pixels, Pixels) {
        (self.line.ascent, self.line.descent)
    }

    pub(super) fn width(&self) -> Pixels {
        self.x_at(self.count())
    }

    pub(super) fn count(&self) -> usize {
        self.glyphs().count()
    }

    pub(super) fn x_at(
        &self,
        index: usize,
    ) -> Pixels {
        let shaped = self
            .glyphs()
            .nth(index)
            .map_or(self.line.width, |(_, glyph)| glyph.position.x);
        shaped + self.spacing * glyphs_before(index)
    }

    pub(super) fn kept_within(
        &self,
        room: Pixels,
    ) -> usize {
        (0..self.count())
            .take_while(|index| self.x_at(index + 1) - self.spacing <= room)
            .count()
    }

    pub(super) fn paint(
        &self,
        count: usize,
        origin: Point<Pixels>,
        window: &mut Window,
    ) {
        for (index, placed) in self.glyphs().take(count).enumerate() {
            let x = origin.x + placed.1.position.x + self.spacing * glyphs_before(index);
            let at = gpui::point(x, origin.y + placed.1.position.y);
            log_failure(self.paint_glyph(at, placed, window));
        }
    }

    fn glyphs(&self) -> impl Iterator<Item = (FontId, &ShapedGlyph)> {
        self.line
            .runs
            .iter()
            .flat_map(|run| run.glyphs.iter().map(move |glyph| (run.font_id, glyph)))
    }

    fn paint_glyph(
        &self,
        at: Point<Pixels>,
        placed: (FontId, &ShapedGlyph),
        window: &mut Window,
    ) -> gpui::Result<()> {
        let (font_id, glyph) = placed;
        if glyph.is_emoji {
            return window.paint_emoji(at, font_id, glyph.id, self.line.font_size);
        }
        window.paint_glyph(at, font_id, glyph.id, self.line.font_size, self.color)
    }
}

fn glyphs_before(index: usize) -> f32 {
    u16::try_from(index).map_or(f32::from(u16::MAX), f32::from)
}

fn log_failure(painted: gpui::Result<()>) {
    if let Err(error) = painted {
        tracing::warn!(%error, "painting a tracked glyph failed");
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, div, px};

    use super::*;
    use crate::test_support::probe::open;

    const SAMPLE: &str = "Crowbar";
    const SIZE: f32 = 20.0;
    const EM: f32 = 0.1;
    const ROOM_CASES: [f32; 5] = [0.0, 13.0, 40.0, 75.0, 400.0];

    fn kept_and_bounds(
        room: Pixels,
        window: &Window,
    ) -> (usize, Pixels, Pixels) {
        let style = TextStyle {
            font_size: px(SIZE).into(),
            ..window.text_style()
        };
        let line = TrackedLine::shape(SAMPLE.into(), &style, EM, window);
        let kept = line.kept_within(room);
        let next = (kept < line.count()).then(|| line.x_at(kept + 1) - line.spacing);
        (
            kept,
            line.x_at(kept) - line.spacing,
            next.unwrap_or(Pixels::MAX),
        )
    }

    #[gpui::test]
    fn a_line_keeps_the_glyphs_that_end_within_its_room(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| div());
        for room in ROOM_CASES.map(px) {
            let (kept, last_end, next_end) = cx.update(|window, _| kept_and_bounds(room, window));
            assert!(
                (kept == 0 || last_end <= room) && next_end > room,
                "{room:?}: {kept} kept, ending at {last_end:?}, the next at {next_end:?}"
            );
        }
    }
}
