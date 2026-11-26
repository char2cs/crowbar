//! Builds a drawable [`Row`] from one grid line.

use std::ops::Range;

use alacritty_terminal::grid::Row as GridRow;
use alacritty_terminal::index::Column;
use alacritty_terminal::term::cell::{Cell, Flags};

use crate::snapshot::palette::ColorSource;
use crate::snapshot::palette::Rgb;
use crate::snapshot::palette::cell_colors;
use crate::snapshot::row::Row;
use crate::snapshot::row::background_span::BackgroundSpan;
use crate::snapshot::row::segment::Segment;
use crate::snapshot::row::style_flags::StyleFlags;
use crate::snapshot::row::style_span::StyleSpan;
use crate::snapshot::row::underline::Underline;

const WIDE_COLUMNS: usize = 2;

pub(crate) struct Builder<'a> {
    colors: ColorSource<'a>,
    selected: Range<usize>,
    selected_style: Option<(StyleFlags, Underline)>,
    row: Row,
    current: Option<Segment>,
}

impl<'a> Builder<'a> {
    pub(crate) fn build(
        colors: ColorSource<'a>,
        cells: &GridRow<Cell>,
        columns: usize,
        selected: Range<usize>,
    ) -> Row {
        let mut builder = Self {
            colors,
            selected,
            selected_style: None,
            row: Row::default(),
            current: None,
        };
        for column in 0..columns {
            builder.push_cell(column, &cells[Column(column)]);
        }
        builder.flush();
        builder.row
    }

    fn push_cell(
        &mut self,
        column: usize,
        cell: &Cell,
    ) {
        let flags = cell.flags;
        if flags.intersects(Flags::WIDE_CHAR_SPACER | Flags::LEADING_WIDE_CHAR_SPACER) {
            return;
        }
        let width = cell_columns(flags);
        let (foreground, background) = self.colors_at(column, cell);
        self.push_background(column, width, background);
        self.push_text(column, cell, foreground);
    }

    fn colors_at(
        &self,
        column: usize,
        cell: &Cell,
    ) -> (Rgb, Rgb) {
        let palette = self.colors.palette;
        if self.selected.contains(&column) {
            return (palette.foreground, palette.background);
        }
        cell_colors(&self.colors, cell)
    }

    fn push_background(
        &mut self,
        column: usize,
        width: usize,
        color: Rgb,
    ) {
        if color == self.colors.palette.background {
            return;
        }
        let touching = |last: &&mut BackgroundSpan| {
            last.color == color && last.column + last.columns == column
        };
        if let Some(last) = self.row.backgrounds.last_mut().filter(touching) {
            last.columns += width;
            return;
        }
        self.row.backgrounds.push(BackgroundSpan {
            column,
            columns: width,
            color,
        });
    }

    fn push_text(
        &mut self,
        column: usize,
        cell: &Cell,
        foreground: Rgb,
    ) {
        let width = cell_columns(cell.flags);
        let continues = self
            .current
            .as_ref()
            .is_some_and(|segment| segment.column + segment.columns == column);
        if width > 1 || !continues {
            self.flush();
        }
        let text = glyph_text(cell);
        let (flags, underline) = self.style_at(column, cell.flags);
        let span = StyleSpan {
            len: text.len(),
            foreground,
            flags,
            underline,
        };
        let segment = self.current.get_or_insert_with(|| Segment {
            column,
            columns: 0,
            glyph_columns: width,
            text: String::new(),
            spans: Vec::new(),
        });
        segment.text.push_str(&text);
        segment.columns += width;
        append_span(segment, span);
        if width > 1 {
            self.flush();
        }
    }

    fn style_at(
        &mut self,
        column: usize,
        flags: Flags,
    ) -> (StyleFlags, Underline) {
        let own = (style_flags(flags), underline_of(flags));
        if !self.selected.contains(&column) {
            return own;
        }
        // SPEC: https://github.com/xtermjs/xterm.js/blob/6.0.0/src/browser/renderer/dom/DomRendererRowFactory.ts
        *self.selected_style.get_or_insert(own)
    }

    fn flush(&mut self) {
        let Some(mut segment) = self.current.take() else {
            return;
        };
        if !segment.trim_blanks() {
            self.row.segments.push(segment);
        }
    }
}

pub(crate) fn cell_columns(flags: Flags) -> usize {
    if flags.contains(Flags::WIDE_CHAR) {
        return WIDE_COLUMNS;
    }
    1
}

fn append_span(
    segment: &mut Segment,
    span: StyleSpan,
) {
    let same_style = |last: &&mut StyleSpan| {
        last.foreground == span.foreground
            && last.flags == span.flags
            && last.underline == span.underline
    };
    if let Some(last) = segment.spans.last_mut().filter(same_style) {
        last.len += span.len;
        return;
    }
    segment.spans.push(span);
}

fn glyph_text(cell: &Cell) -> String {
    let hidden = cell.flags.contains(Flags::HIDDEN);
    let glyph = match (hidden, cell.c) {
        (true, _) | (false, '\0') => ' ',
        (false, other) => other,
    };
    let marks = cell.zerowidth().into_iter().flatten().copied();
    std::iter::once(glyph).chain(marks).collect()
}

fn style_flags(flags: Flags) -> StyleFlags {
    let mut style = StyleFlags::NONE;
    if flags.contains(Flags::BOLD) {
        style = style.with(StyleFlags::BOLD);
    }
    if flags.contains(Flags::ITALIC) {
        style = style.with(StyleFlags::ITALIC);
    }
    if flags.contains(Flags::STRIKEOUT) {
        style = style.with(StyleFlags::STRIKE);
    }
    style
}

fn underline_of(flags: Flags) -> Underline {
    const KINDS: [(Flags, Underline); 5] = [
        (Flags::UNDERCURL, Underline::Curly),
        (Flags::DOUBLE_UNDERLINE, Underline::Double),
        (Flags::DOTTED_UNDERLINE, Underline::Dotted),
        (Flags::DASHED_UNDERLINE, Underline::Dashed),
        (Flags::UNDERLINE, Underline::Single),
    ];
    KINDS
        .iter()
        .find(|(flag, _)| flags.contains(*flag))
        .map_or(Underline::None, |(_, kind)| *kind)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn underline_prefers_the_specific_variant() {
        let cases = [
            (Flags::UNDERCURL | Flags::UNDERLINE, Underline::Curly),
            (Flags::UNDERLINE, Underline::Single),
            (Flags::empty(), Underline::None),
        ];
        for (flags, expected) in cases {
            assert_eq!(underline_of(flags), expected, "{flags:?}");
        }
    }
}
