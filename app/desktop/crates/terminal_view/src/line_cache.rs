//! Shaped text kept between frames: only rows whose content changed are shaped again.

use gpui::{ShapedLine, SharedString, TextRun, UnderlineStyle, Window, px};
use terminal::{Row, Segment, StyleFlags, StyleSpan, Underline};

use crate::cached_row::CachedRow;
use crate::shaping::Shaping;
use crate::terminal_font::styled_font;
use crate::theme_palette::hsla_of;

const LINE_THICKNESS: f32 = 1.0;

#[derive(Default)]
pub(crate) struct LineCache {
    rows: Vec<Option<CachedRow>>,
}

impl LineCache {
    pub(crate) fn clear(&mut self) {
        self.rows.clear();
    }

    pub(crate) fn lines(
        &mut self,
        index: usize,
        row: &Row,
        shaping: &Shaping,
        window: &Window,
    ) -> &[ShapedLine] {
        if self.rows.len() <= index {
            self.rows.resize_with(index + 1, || None);
        }
        let slot = self.rows.get_mut(index);
        let Some(slot) = slot else {
            return &[];
        };
        if slot.as_ref().is_none_or(|cached| cached.row != *row) {
            *slot = Some(shaped_row(row, shaping, window));
        }
        slot.as_ref().map_or(&[], |cached| cached.lines.as_slice())
    }
}

fn shaped_row(
    row: &Row,
    shaping: &Shaping,
    window: &Window,
) -> CachedRow {
    CachedRow {
        row: row.clone(),
        lines: shape_row(row, shaping, window),
    }
}

fn shape_row(
    row: &Row,
    shaping: &Shaping,
    window: &Window,
) -> Vec<ShapedLine> {
    row.segments
        .iter()
        .map(|segment| shape_segment(segment, shaping, window))
        .collect()
}

fn shape_segment(
    segment: &Segment,
    shaping: &Shaping,
    window: &Window,
) -> ShapedLine {
    let runs: Vec<TextRun> = segment
        .spans
        .iter()
        .map(|span| text_run(span, shaping))
        .collect();
    let glyph_width = shaping.cell_width * segment.glyph_columns as f32;
    window.text_system().shape_line(
        SharedString::from(segment.text.clone()),
        shaping.font_size,
        &runs,
        Some(glyph_width),
    )
}

fn text_run(
    span: &StyleSpan,
    shaping: &Shaping,
) -> TextRun {
    let color = hsla_of(span.foreground);
    let line = UnderlineStyle {
        thickness: px(LINE_THICKNESS),
        color: Some(color),
        wavy: span.underline == Underline::Curly,
    };
    let strike = gpui::StrikethroughStyle {
        thickness: px(LINE_THICKNESS),
        color: Some(color),
    };
    TextRun {
        len: span.len,
        font: styled_font(&shaping.font, span.flags),
        color,
        background_color: None,
        underline: (span.underline != Underline::None).then_some(line),
        strikethrough: span.flags.contains(StyleFlags::STRIKE).then_some(strike),
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use terminal::{CursorShape, DaemonFrame, Rgb, Terminal, TerminalConfig, TerminalPalette};

    use super::*;
    use crate::terminal_font::base_font;
    use crate::terminal_settings::TerminalSettings;
    use crate::test_support::open;

    fn first_row(output: &str) -> Row {
        let gray = Rgb::from_hex(0x80_8080);
        let palette = TerminalPalette {
            foreground: gray,
            background: Rgb::from_hex(0),
            cursor: gray,
            ansi: [gray; 16],
        };
        let mut terminal = Terminal::new(TerminalConfig {
            columns: 20,
            lines: 2,
            scrollback: 0,
            cursor_shape: CursorShape::Block,
            cursor_blink: false,
            palette,
        });
        terminal.apply(&DaemonFrame::Output(output.as_bytes().to_vec()));
        terminal
            .snapshot()
            .rows
            .into_iter()
            .next()
            .unwrap_or_default()
    }

    #[gpui::test]
    fn one_line_is_shaped_per_segment_and_clear_forgets_them(cx: &mut TestAppContext) {
        let (_, cx) = open(cx);
        let settings = TerminalSettings::default();
        let shaping = Shaping {
            font: base_font(&settings),
            font_size: px(settings.font_size),
            cell_width: px(8.0),
        };
        let mut cache = LineCache::default();
        let row = first_row("ab\x1b[1mcd");
        let shaped = cx.update(|window, _| cache.lines(0, &row, &shaping, window).len());
        assert_eq!(shaped, row.segments.len());
        cache.clear();
        assert!(cache.rows.is_empty());
    }
}
