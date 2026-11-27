use gpui_kit::base::input::Rope;
use lsp_types::TextEdit;
use serde_json::Value;

use crate::lsp::lsp_position::offset_of;

pub(crate) fn formatted(
    text: &Rope,
    answer: Value,
) -> Option<String> {
    let mut edits: Vec<TextEdit> = serde_json::from_value(answer).ok()?;
    edits.sort_by_key(|edit| std::cmp::Reverse((edit.range.start, edit.range.end)));
    let formatted = edits.into_iter().fold(text.to_string(), |so_far, edit| {
        applied(so_far, text, &edit)
    });
    (formatted != *text).then_some(formatted)
}

fn applied(
    mut so_far: String,
    text: &Rope,
    edit: &TextEdit,
) -> String {
    let range = offset_of(text, edit.range.start)..offset_of(text, edit.range.end);
    if so_far.get(range.clone()).is_none() {
        return so_far;
    }
    so_far.replace_range(range, &edit.new_text);
    so_far
}

#[cfg(test)]
mod tests {
    use lsp_types::{Position, Range};
    use serde_json::json;

    use super::*;

    fn edit(
        from: (u32, u32),
        to: (u32, u32),
        text: &str,
    ) -> Value {
        let at = |(line, character)| Position::new(line, character);
        json!(TextEdit::new(Range::new(at(from), at(to)), text.to_owned()))
    }

    #[test]
    fn edits_apply_against_the_unedited_text_and_count_utf16_units() {
        let text = Rope::from("😀 =  1\nb\n");
        let answer = json!([edit((0, 4), (0, 6), " "), edit((1, 0), (1, 1), "c")]);
        assert_eq!(formatted(&text, answer).as_deref(), Some("😀 = 1\nc\n"));
    }

    #[test]
    fn nothing_to_change_or_no_answer_formats_nothing() {
        let text = Rope::from("a\n");
        for answer in [Value::Null, json!([]), json!([edit((0, 0), (0, 1), "a")])] {
            assert_eq!(formatted(&text, answer.clone()), None, "{answer}");
        }
    }
}
