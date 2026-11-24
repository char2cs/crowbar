use std::ops::Range;

use crate::Vertical;

mod follow;
mod line_block;
mod line_table;
mod rewrite;

use follow::Follow;
use line_block::LineBlock;
use line_table::LineTable;
use rewrite::Rewrite;

/// One replacement over whole lines that a line command makes (Monaco's move, copy and delete
/// line actions), with where the selections go after it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LineEdit {
    range: Range<usize>,
    text: String,
    selections: Vec<Range<usize>>,
}

impl LineEdit {
    /// The lines under `selections` moved one line `toward`, swapping with the line beside them
    /// (alt+Up / alt+Down); `None` when a block already touches that end of the text.
    #[must_use]
    pub fn moved(
        text: &str,
        selections: &[Range<usize>],
        toward: Vertical,
    ) -> Option<Self> {
        let table = LineTable::of(text);
        let blocks = LineBlock::merged(&table, selections);
        let at_edge = |block: &LineBlock| match toward {
            Vertical::Up => block.first == 0,
            Vertical::Down => block.last == table.last(),
        };
        if blocks.iter().any(at_edge) {
            return None;
        }
        let rewrite = Rewrite::moved(&table, &blocks, toward)?;
        Some(Self::of(
            &rewrite,
            rewrite.placed(selections, Follow::First),
        ))
    }

    /// The lines under `selections` copied beside themselves; the selections follow the copy
    /// `toward` (Monaco's copy line up and down).
    #[must_use]
    pub fn copied(
        text: &str,
        selections: &[Range<usize>],
        toward: Vertical,
    ) -> Option<Self> {
        let table = LineTable::of(text);
        let blocks = LineBlock::merged(&table, selections);
        let rewrite = Rewrite::copied(&table, &blocks)?;
        let follows = match toward {
            Vertical::Up => Follow::First,
            Vertical::Down => Follow::Last,
        };
        Some(Self::of(&rewrite, rewrite.placed(selections, follows)))
    }

    /// The lines under `selections` deleted with their line breaks, each caret on the line that
    /// takes their place at its old column (ctrl+shift+k).
    #[must_use]
    pub fn deleted(
        text: &str,
        selections: &[Range<usize>],
    ) -> Option<Self> {
        let table = LineTable::of(text);
        let blocks = LineBlock::merged(&table, selections);
        let rewrite = Rewrite::deleted(&table, &blocks)?;
        let carets = selections
            .iter()
            .map(|selection| rewrite.caret_after_delete(selection, &blocks))
            .collect();
        Some(Self::of(&rewrite, carets))
    }

    fn of(
        rewrite: &Rewrite<'_, '_>,
        selections: Vec<Range<usize>>,
    ) -> Self {
        Self {
            range: rewrite.range(),
            text: rewrite.text(),
            selections,
        }
    }

    /// The text range the edit replaces.
    #[must_use]
    pub fn range(&self) -> Range<usize> {
        self.range.clone()
    }

    /// The text that replaces the range.
    #[must_use]
    pub fn text(&self) -> &str {
        &self.text
    }

    /// The selections after the edit, as offsets into the edited text.
    #[must_use]
    pub fn selections(&self) -> &[Range<usize>] {
        &self.selections
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Clone, Copy, Debug)]
    enum Command {
        Move(Vertical),
        Copy(Vertical),
        Delete,
    }

    fn applied(
        text: &str,
        edit: &LineEdit,
    ) -> String {
        let mut edited = text.to_owned();
        edited.replace_range(edit.range(), edit.text());
        edited
    }

    fn run(
        command: Command,
        text: &str,
        selections: &[Range<usize>],
    ) -> Option<LineEdit> {
        match command {
            Command::Move(toward) => LineEdit::moved(text, selections, toward),
            Command::Copy(toward) => LineEdit::copied(text, selections, toward),
            Command::Delete => LineEdit::deleted(text, selections),
        }
    }

    type Spans = &'static [(usize, usize)];

    struct Case {
        command: Command,
        text: &'static str,
        selections: Spans,
        expected: &'static str,
        carets: Spans,
    }

    const fn case(
        command: Command,
        before: (&'static str, Spans),
        after: (&'static str, Spans),
    ) -> Case {
        Case {
            command,
            text: before.0,
            selections: before.1,
            expected: after.0,
            carets: after.1,
        }
    }

    const DOWN: Command = Command::Move(Vertical::Down);
    const UP: Command = Command::Move(Vertical::Up);

    const CASES: &[Case] = &[
        case(DOWN, ("a\nbb\nc", &[(1, 1)]), ("bb\na\nc", &[(4, 4)])),
        case(UP, ("a\nbb\nc", &[(3, 4)]), ("bb\na\nc", &[(1, 2)])),
        case(UP, ("a\nb\nc\nd", &[(2, 5)]), ("b\nc\na\nd", &[(0, 3)])),
        case(DOWN, ("a\nb\nc", &[(0, 2)]), ("b\na\nc", &[(2, 4)])),
        case(
            Command::Copy(Vertical::Down),
            ("ab\nc", &[(1, 1)]),
            ("ab\nab\nc", &[(4, 4)]),
        ),
        case(
            Command::Copy(Vertical::Up),
            ("ab\nc", &[(1, 1)]),
            ("ab\nab\nc", &[(1, 1)]),
        ),
        case(
            Command::Delete,
            ("a\nbb\nc", &[(3, 3)]),
            ("a\nc", &[(3, 3)]),
        ),
        case(Command::Delete, ("a\nbb", &[(3, 4)]), ("a", &[(1, 1)])),
        case(Command::Delete, ("only", &[(2, 2)]), ("", &[(0, 0)])),
        case(
            Command::Delete,
            ("a\nb\nc\nd", &[(0, 0), (4, 4)]),
            ("b\nd", &[(0, 0), (2, 2)]),
        ),
    ];

    fn ranges(spans: Spans) -> Vec<Range<usize>> {
        spans.iter().map(|(start, end)| *start..*end).collect()
    }

    #[test]
    fn line_commands_rewrite_the_lines_under_the_selections_and_carry_the_carets() {
        for case in CASES {
            let (command, text) = (case.command, case.text);
            let edit = run(command, text, &ranges(case.selections));
            let edit = edit.unwrap_or_else(|| panic!("{command:?} on {text:?} did nothing"));
            assert_eq!(applied(text, &edit), case.expected, "{command:?} {text:?}");
            assert_eq!(
                edit.selections(),
                ranges(case.carets),
                "{command:?} {text:?}"
            );
        }
    }

    #[test]
    fn a_block_at_the_edge_it_would_move_past_stays() {
        assert_eq!(
            LineEdit::moved("a\nb", std::slice::from_ref(&(0..0)), Vertical::Up),
            None
        );
        assert_eq!(
            LineEdit::moved("a\nb", std::slice::from_ref(&(2..2)), Vertical::Down),
            None
        );
    }
}
