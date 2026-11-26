//! LSP positions count UTF-16 code units, as develop's Monaco sends them and the daemon passes
//! them through; the editor's rope counts chars. Every position crosses here on its way to or
//! from a language server.

use gpui_kit::base::input::{Rope, RopeExt as _};
use lsp_types::{Position, Range};

pub(crate) fn to_lsp(
    text: &Rope,
    offset: usize,
) -> Position {
    let point = text.offset_to_point(offset);
    let line = text.slice_line(point.row);
    let units = line.byte_to_utf16_idx(point.column.min(line.len()));
    Position::new(clamped(point.row), clamped(units))
}

pub(crate) fn from_lsp(
    text: &Rope,
    position: Position,
) -> Position {
    let line = text.slice_line(position.line as usize);
    let units = (position.character as usize).min(line.len_utf16());
    let chars = line.byte_to_char_idx(line.utf16_to_byte_idx(units));
    Position::new(position.line, clamped(chars))
}

pub(crate) fn range_from_lsp(
    text: &Rope,
    range: Range,
) -> Range {
    Range::new(from_lsp(text, range.start), from_lsp(text, range.end))
}

fn clamped(count: usize) -> u32 {
    u32::try_from(count).unwrap_or(u32::MAX)
}

#[cfg(test)]
mod tests {
    use super::*;

    const LINES: &str = "ab\nlet a = \"😀\"; a\n";

    #[test]
    fn a_position_past_an_astral_char_counts_two_units_for_it_both_ways() {
        let text = Rope::from(LINES);
        let cases = [
            (0, (0, 0), (0, 0)),
            (16, (1, 10), (1, 11)),
            (19, (1, 13), (1, 14)),
        ];
        for (offset, (line, chars), (lsp_line, units)) in cases {
            let lsp = Position::new(lsp_line, units);
            assert_eq!(to_lsp(&text, offset), lsp, "{offset}");
            assert_eq!(from_lsp(&text, lsp), Position::new(line, chars), "{offset}");
        }
    }

    #[test]
    fn a_position_past_the_line_end_stops_at_the_line_end() {
        let text = Rope::from(LINES);
        assert_eq!(from_lsp(&text, Position::new(0, 9)), Position::new(0, 2));
    }
}
