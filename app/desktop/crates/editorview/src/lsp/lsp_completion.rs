//! Completion from the document's language server, as develop's Monaco completion provider
//! asks it: while typing a word, after one of its trigger characters, and on ctrl+space.

use std::rc::Rc;

use gpui::{App, AppContext as _, Task, Window};
use gpui_kit::base::input::{CompletionProvider, Rope, RopeExt as _};
use lsp_types::{
    CompletionContext, CompletionItem, CompletionResponse, CompletionTextEdit, InsertReplaceEdit,
    Range, TextEdit,
};
use serde_json::{Map, json};

use crate::lsp::lsp_link::LspLink;
use crate::lsp::lsp_position::{range_from_lsp, to_lsp};

const TRIGGER_CHARACTERS: [char; 9] = ['.', ':', '<', '"', '\'', '/', '@', '>', '#'];

pub(crate) struct LspCompletion {
    pub(crate) link: Rc<LspLink>,
}

impl CompletionProvider for LspCompletion {
    fn completions(
        &self,
        text: &Rope,
        offset: usize,
        _trigger: CompletionContext,
        _window: &mut Window,
        cx: &mut App,
    ) -> Task<gpui::Result<CompletionResponse>> {
        let mut fields = Map::new();
        fields.insert("position".to_owned(), json!(to_lsp(text, offset)));
        let asked = self.link.request("completion", fields, cx);
        let text = text.clone();
        cx.background_spawn(async move {
            let answer = asked.await?;
            let response: Option<CompletionResponse> =
                serde_json::from_value(answer).ok().flatten();
            let word = word_until(&text, offset);
            let items = response.map_or_else(Vec::new, items_of);
            let edited = items.into_iter().map(|item| edited(item, (&text, word)));
            Ok(CompletionResponse::Array(edited.collect()))
        })
    }

    fn is_completion_trigger(
        &self,
        _offset: usize,
        new_text: &str,
        _cx: &mut App,
    ) -> bool {
        new_text
            .chars()
            .last()
            .is_some_and(|typed| is_word(typed) || TRIGGER_CHARACTERS.contains(&typed))
    }
}

fn items_of(response: CompletionResponse) -> Vec<CompletionItem> {
    match response {
        CompletionResponse::Array(items) => items,
        CompletionResponse::List(list) => list.items,
    }
}

fn word_until(
    text: &Rope,
    offset: usize,
) -> Range {
    let typed: usize = text
        .chars_at(offset)
        .reversed()
        .take_while(|typed| is_word(*typed))
        .map(char::len_utf8)
        .sum();
    let start = text.offset_to_position(offset.saturating_sub(typed));
    Range::new(start, text.offset_to_position(offset))
}

fn edited(
    item: CompletionItem,
    (text, word): (&Rope, Range),
) -> CompletionItem {
    let edit = match item.text_edit.clone() {
        Some(edit) => in_chars(text, edit),
        None => CompletionTextEdit::Edit(TextEdit {
            range: word,
            new_text: item
                .insert_text
                .clone()
                .unwrap_or_else(|| item.label.clone()),
        }),
    };
    CompletionItem {
        text_edit: Some(edit),
        ..item
    }
}

fn in_chars(
    text: &Rope,
    edit: CompletionTextEdit,
) -> CompletionTextEdit {
    match edit {
        CompletionTextEdit::Edit(edit) => CompletionTextEdit::Edit(TextEdit {
            range: range_from_lsp(text, edit.range),
            new_text: edit.new_text,
        }),
        CompletionTextEdit::InsertAndReplace(edit) => {
            CompletionTextEdit::InsertAndReplace(both_in_chars(text, edit))
        },
    }
}

fn both_in_chars(
    text: &Rope,
    edit: InsertReplaceEdit,
) -> InsertReplaceEdit {
    InsertReplaceEdit {
        new_text: edit.new_text,
        insert: range_from_lsp(text, edit.insert),
        replace: range_from_lsp(text, edit.replace),
    }
}

fn is_word(typed: char) -> bool {
    typed.is_alphanumeric() || typed == '_' || typed == '$'
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use lsp_types::Position;
    use serde_json::json;

    use super::*;
    use crate::editor_view::test_support::{completions_at, served};

    fn edit_range(item: &CompletionItem) -> Option<Range> {
        match item.text_edit.as_ref()? {
            CompletionTextEdit::Edit(edit) => Some(edit.range),
            CompletionTextEdit::InsertAndReplace(edit) => Some(edit.replace),
        }
    }

    #[gpui::test]
    fn completion_past_an_emoji_asks_in_utf16_and_edits_in_chars(cx: &mut TestAppContext) {
        let (view, window, server) = served(("src/a.ts", "s = \"😀\"; ab"), cx);
        let range =
            json!({"start": {"line": 0, "character": 10}, "end": {"line": 0, "character": 12}});
        let items = json!({"isIncomplete": false, "items": [
            {"label": "abc"},
            {"label": "abd", "textEdit": {"range": range, "newText": "abd"}},
        ]});
        server.answer("completion", items);
        let items = completions_at(&view, 14, window);
        let position = json!({"path": "src/a.ts", "position": {"line": 0, "character": 12}});
        assert_eq!(server.bodies()[1], position);
        let word = Range::new(Position::new(0, 9), Position::new(0, 11));
        let ranges: Vec<Option<Range>> = items.iter().map(edit_range).collect();
        assert_eq!(ranges, [Some(word), Some(word)]);
    }

    #[gpui::test]
    fn ctrl_space_asks_the_server_for_completions(cx: &mut TestAppContext) {
        let (_view, window, server) = served(("src/a.ts", "a"), cx);
        window.simulate_keystrokes("ctrl-space");
        window.run_until_parked();
        assert_eq!(server.verbs(), ["didOpen", "completion"]);
    }

    #[gpui::test]
    fn typing_a_word_or_a_trigger_character_sends_the_edit_then_asks_and_a_space_does_not(
        cx: &mut TestAppContext
    ) {
        let asks = ["didOpen", "didChange", "completion"].as_slice();
        let waits = ["didOpen"].as_slice();
        let cases = [
            ("a", asks),
            (".", asks),
            ("$", asks),
            (" ", waits),
            ("(", waits),
        ];
        for (typed, expected) in cases {
            let (_view, window, server) = served(("src/a.ts", ""), cx);
            window.simulate_input(typed);
            window.run_until_parked();
            assert_eq!(server.verbs(), expected, "{typed}");
        }
    }
}
