use gpui::{App, Pixels, Point, TextAlign, Window, point, px};

use super::{TrackedLine, glyphs_before};

impl TrackedLine {
    pub(crate) fn x_for_index(
        &self,
        index: usize,
    ) -> Pixels {
        let before = self.glyphs().filter(|(_, glyph)| glyph.index < index);
        self.line.x_for_index(index) + self.spacing * glyphs_before(before.count())
    }

    pub(crate) fn closest_index_for_x(
        &self,
        x: Pixels,
    ) -> usize {
        let distance = |index: usize| f32::from(self.x_for_index(index) - x).abs();
        let starts = self.glyphs().map(|(_, glyph)| glyph.index);
        starts
            .chain([self.line.len])
            .min_by(|a, b| distance(*a).total_cmp(&distance(*b)))
            .unwrap_or_default()
    }

    pub(crate) fn index_for_x(
        &self,
        x: Pixels,
    ) -> Option<usize> {
        if x < px(0.0) || x >= self.width() {
            return None;
        }
        let starts = self.glyphs().map(|(_, glyph)| glyph.index);
        starts
            .take_while(|index| self.x_for_index(*index) <= x)
            .last()
    }

    pub(crate) fn paint_line(
        &self,
        origin: Point<Pixels>,
        window: &mut Window,
        cx: &mut App,
    ) -> gpui::Result<()> {
        let line_height = window.line_height();
        if self.spacing == px(0.0) {
            return self
                .line
                .paint(origin, line_height, TextAlign::Left, None, window, cx);
        }
        let (ascent, descent) = self.extent();
        let padding = (line_height - ascent - descent) / 2.0;
        let baseline = point(origin.x, origin.y + padding + ascent);
        self.paint(self.count(), baseline, window);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, TextStyle, px};

    use super::*;

    #[gpui::test]
    fn each_boundary_sits_after_the_spacing_of_the_glyphs_before_it(cx: &mut TestAppContext) {
        let cx = cx.add_empty_window();
        let (spaced, plain) = cx.update(|window, _| {
            let style = TextStyle::default();
            let shape = |em| TrackedLine::shape("Name".into(), &style, em, window);
            (shape(0.1), shape(0.0))
        });
        let spacing = spaced.x_for_index(2) - plain.x_for_index(2);
        assert!(spacing > px(0.0), "two glyphs are spaced");
        assert_eq!(spaced.closest_index_for_x(spaced.x_for_index(3)), 3);
        assert_eq!(spaced.index_for_x(spaced.x_for_index(1)), Some(1));
        assert_eq!(spaced.index_for_x(spaced.width()), None);
    }
}
