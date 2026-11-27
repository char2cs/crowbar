mod typed_number;

use typed_number::{clamp_signed, leading_number, signed};

const PREFIX: char = ':';
const SEPARATORS: [char; 3] = [',', ':', '#'];

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct GotoTarget {
    position: Option<(usize, usize)>,
    label: String,
}

impl GotoTarget {
    #[must_use]
    pub fn parse(
        value: &str,
        text: &str,
    ) -> Self {
        let value = value.trim();
        let value = value.strip_prefix(PREFIX).unwrap_or(value);
        match value.strip_prefix(PREFIX) {
            Some(offset) => Self::at_offset(offset, text),
            None => Self::at_line(value, text),
        }
    }

    #[must_use]
    pub fn position(&self) -> Option<(usize, usize)> {
        self.position
    }

    #[must_use]
    pub fn label(&self) -> &str {
        &self.label
    }

    #[must_use]
    pub fn offset_in(
        &self,
        text: &str,
    ) -> Option<usize> {
        let (line, column) = self.position?;
        let start: usize = text
            .split('\n')
            .take(line - 1)
            .map(|line| line.len() + 1)
            .sum();
        let line_text = text.split('\n').nth(line - 1)?;
        let within: usize = line_text.chars().take(column - 1).map(char::len_utf8).sum();
        Some(start + within)
    }

    fn at_line(
        value: &str,
        text: &str,
    ) -> Self {
        let parts: Vec<&str> = value.split(SEPARATORS).collect();
        let max_line = text.split('\n').count();
        let Some(asked) = parts.first().and_then(|part| leading_number(part)) else {
            return Self::unplaced(format!(
                "Type a line number to go to (from 1 to {max_line})."
            ));
        };
        let line = clamp_signed(asked, max_line + 1, max_line);
        let max_column = column_count(text, line) + 1;
        let column = parts.get(1).and_then(|part| leading_number(part));
        let Some(column) = column else {
            return Self::placed((line, 1), line_label(line, parts.len(), max_column));
        };
        let column = clamp_signed(column, max_column, max_column);
        Self::reached(line, column)
    }

    fn at_offset(
        value: &str,
        text: &str,
    ) -> Self {
        let length = text.chars().count();
        let Some(offset) = leading_number(value) else {
            return Self::unplaced(format!(
                "Type a character position to go to (from 1 to {length})."
            ));
        };
        let zero_based = offset - offset.signum();
        let from_start = zero_based + i64::from(zero_based < 0) * signed(length);
        let offset = usize::try_from(from_start.max(0)).unwrap_or(0).min(length);
        let before: String = text.chars().take(offset).collect();
        let line = before.matches('\n').count() + 1;
        let column = before
            .rsplit('\n')
            .next()
            .map_or(0, |tail| tail.chars().count())
            + 1;
        Self::reached(line, column)
    }

    fn reached(
        line: usize,
        column: usize,
    ) -> Self {
        let label = format!("Press 'Enter' to go to line {line} at column {column}.");
        Self::placed((line, column), label)
    }

    fn placed(
        position: (usize, usize),
        label: String,
    ) -> Self {
        Self {
            position: Some(position),
            label,
        }
    }

    fn unplaced(label: String) -> Self {
        Self {
            position: None,
            label,
        }
    }
}

fn line_label(
    line: usize,
    parts: usize,
    max_column: usize,
) -> String {
    if parts == 1 {
        return format!("Press 'Enter' to go to line {line} or enter : to add a column number.");
    }
    format!("Press 'Enter' to go to line {line} or enter a column number (from 1 to {max_column}).")
}

fn column_count(
    text: &str,
    line: usize,
) -> usize {
    text.split('\n')
        .nth(line - 1)
        .map_or(0, |line| line.chars().count())
}

#[cfg(test)]
mod tests {
    use super::*;

    type Position = Option<(usize, usize)>;

    const TEXT: &str = "one\ntwo words\nthree";
    const ADD_COLUMN: &str = "or enter : to add a column number.";
    const CASES: &[(&str, Position, &str)] = &[
        (":", None, "Type a line number to go to (from 1 to 3)."),
        (":x", None, "Type a line number to go to (from 1 to 3)."),
        (":2", Some((2, 1)), "Press 'Enter' to go to line 2 "),
        (
            ":2:",
            Some((2, 1)),
            "Press 'Enter' to go to line 2 or enter a column number (from 1 to 10).",
        ),
        (
            ":2:5",
            Some((2, 5)),
            "Press 'Enter' to go to line 2 at column 5.",
        ),
        (
            ":2,99",
            Some((2, 10)),
            "Press 'Enter' to go to line 2 at column 10.",
        ),
        (":-1", Some((3, 1)), "Press 'Enter' to go to line 3 "),
        (":40", Some((3, 1)), "Press 'Enter' to go to line 3 "),
        (
            "::5",
            Some((2, 1)),
            "Press 'Enter' to go to line 2 at column 1.",
        ),
        (
            "::",
            None,
            "Type a character position to go to (from 1 to 19).",
        ),
    ];

    #[test]
    fn the_typed_position_reads_as_monaco_s_go_to_line() {
        for (value, position, label) in CASES {
            let target = GotoTarget::parse(value, TEXT);
            let label = label
                .strip_suffix(' ')
                .map_or((*label).to_owned(), |line| format!("{line} {ADD_COLUMN}"));
            assert_eq!(target.position(), *position, "{value}");
            assert_eq!(target.label(), label, "{value}");
        }
    }

    #[test]
    fn a_target_s_offset_counts_characters_on_its_line() {
        let text = "a\nébc";
        assert_eq!(GotoTarget::parse(":2:3", text).offset_in(text), Some(5));
    }
}
