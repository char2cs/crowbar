use std::{ops::Range, rc::Rc};

use gpui::{Bounds, Half, Pixels, ShapedLine, TextAlign, px};

use super::{WrappingIndent, display_map::LineLayout};

/// Which spaces and tabs a code editor marks, as Monaco's `renderWhitespace`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum WhitespaceMode {
    /// No markers.
    #[default]
    None,
    /// Every space and tab except a single space between two words.
    Boundary,
    /// The spaces and tabs after the last other character of a line.
    Trailing,
    /// Every space and tab.
    All,
}

impl WhitespaceMode {
    /// Every whitespace character when `show`, none otherwise.
    pub(crate) fn of(show: bool) -> Self {
        if show { Self::All } else { Self::None }
    }

    /// Whether the space or tab at byte `offset` of `line` gets a marker.
    pub(crate) fn marks(self, line: &str, offset: usize) -> bool {
        match self {
            Self::None => false,
            Self::All => true,
            Self::Trailing => offset >= line.trim_end_matches([' ', '\t']).len(),
            Self::Boundary => !is_lone_space_between_words(line, offset),
        }
    }
}

fn is_lone_space_between_words(line: &str, offset: usize) -> bool {
    let is_blank = |c: char| matches!(c, ' ' | '\t');
    let before = line[..offset].chars().next_back();
    let after = line[offset..].chars().nth(1);
    line[offset..].starts_with(' ')
        && before.is_some_and(|c| !is_blank(c))
        && after.is_some_and(|c| !is_blank(c))
}

#[derive(Clone, Default)]
pub(crate) struct WhitespaceIndicators {
    pub(crate) space: ShapedLine,
    pub(crate) tab: ShapedLine,
    pub(crate) mode: WhitespaceMode,
}

#[derive(Clone)]
pub(super) struct LastLayout {
    pub(super) visible_range: Range<usize>,
    pub(super) visible_buffer_lines: Vec<usize>,
    pub(super) visible_line_byte_offsets: Vec<usize>,
    pub(super) visible_top: Pixels,
    pub(super) visible_range_offset: Range<usize>,
    pub(super) lines: Rc<Vec<LineLayout>>,
    pub(super) line_height: Pixels,
    pub(super) wrap_width: Option<Pixels>,
    pub(super) wrapping_indent: WrappingIndent,
    pub(super) line_number_width: Pixels,
    /// Width of one space in the editor font.
    ///
    /// Past the end of a line there are no glyphs to hit-test against, so this is the
    /// step used to measure how far past the end a pointer sits.
    pub(super) space_width: Pixels,
    pub(super) cursor_bounds: Option<Bounds<Pixels>>,
    pub(super) text_align: TextAlign,
    pub(super) content_width: Pixels,
}

impl LastLayout {
    pub(crate) fn line(&self, row: usize) -> Option<&LineLayout> {
        let pos = self.visible_buffer_lines.binary_search(&row).ok()?;
        self.lines.get(pos)
    }

    pub(super) fn alignment_offset(&self, line_width: Pixels) -> Pixels {
        match self.text_align {
            TextAlign::Left => px(0.),
            TextAlign::Center => (self.content_width - line_width).half().max(px(0.)),
            TextAlign::Right => (self.content_width - line_width).max(px(0.)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::WhitespaceMode;

    fn marked(mode: WhitespaceMode, line: &str) -> Vec<usize> {
        line.char_indices()
            .filter(|(offset, c)| matches!(c, ' ' | '\t') && mode.marks(line, *offset))
            .map(|(offset, _)| offset)
            .collect()
    }

    #[test]
    fn each_mode_marks_the_whitespace_monaco_renders() {
        let line = "  a b  c\td ";
        assert_eq!(marked(WhitespaceMode::None, line), Vec::<usize>::new());
        assert_eq!(marked(WhitespaceMode::All, line), [0, 1, 3, 5, 6, 8, 10]);
        assert_eq!(marked(WhitespaceMode::Boundary, line), [0, 1, 5, 6, 8, 10]);
        assert_eq!(marked(WhitespaceMode::Trailing, line), [10]);
        assert_eq!(marked(WhitespaceMode::Trailing, "   "), [0, 1, 2]);
    }
}
