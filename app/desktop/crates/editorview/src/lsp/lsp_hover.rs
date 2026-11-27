use std::rc::Rc;

use gpui::{App, AppContext as _, Task, Window};
use gpui_kit::base::input::{HoverProvider, Rope};
use serde_json::{Map, json};

use crate::lsp::lsp_link::LspLink;
use crate::lsp::lsp_position::{range_from_lsp, to_lsp};

pub(crate) struct LspHover {
    pub(crate) link: Rc<LspLink>,
}

impl HoverProvider for LspHover {
    fn hover(
        &self,
        text: &Rope,
        offset: usize,
        _window: &mut Window,
        cx: &mut App,
    ) -> Task<gpui::Result<Option<lsp_types::Hover>>> {
        let mut fields = Map::new();
        fields.insert("position".to_owned(), json!(to_lsp(text, offset)));
        let asked = self.link.request("hover", fields, cx);
        let text = text.clone();
        cx.background_spawn(async move {
            let answer = asked.await?;
            let hover: Option<lsp_types::Hover> = serde_json::from_value(answer).ok().flatten();
            Ok(hover.map(|hover| in_chars(&text, hover)))
        })
    }
}

fn in_chars(
    text: &Rope,
    hover: lsp_types::Hover,
) -> lsp_types::Hover {
    let range = hover.range.map(|range| range_from_lsp(text, range));
    lsp_types::Hover { range, ..hover }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use lsp_types::{HoverContents, MarkedString};
    use serde_json::json;

    use crate::editor_view::test_support::{hover_at, served};

    #[gpui::test]
    fn a_hover_sends_the_pending_edit_first_and_asks_at_the_offset_s_position(
        cx: &mut TestAppContext
    ) {
        let (view, window, server) = served(("src/a.ts", "let a = 1\nlet b = a"), cx);
        server.answer("hover", json!({"contents": "const a: 1"}));
        window.simulate_input(" ");
        let hover = hover_at(&view, 19, window);
        assert_eq!(server.verbs(), ["didOpen", "didChange", "hover"]);
        let position = json!({"path": "src/a.ts", "position": {"line": 1, "character": 8}});
        assert_eq!(server.bodies()[2], position);
        let shown = MarkedString::String("const a: 1".to_owned());
        assert_eq!(
            hover.map(|hover| hover.contents),
            Some(HoverContents::Scalar(shown))
        );
    }

    #[gpui::test]
    fn a_hover_past_an_emoji_counts_utf16_units_both_ways(cx: &mut TestAppContext) {
        let (view, window, server) = served(("src/a.ts", "let a = \"😀\"; a"), cx);
        let range =
            json!({"start": {"line": 0, "character": 14}, "end": {"line": 0, "character": 15}});
        server.answer(
            "hover",
            json!({"contents": "const a: string", "range": range}),
        );
        let hover = hover_at(&view, 16, window);
        let position = json!({"path": "src/a.ts", "position": {"line": 0, "character": 14}});
        assert_eq!(server.bodies()[1], position);
        let chars = lsp_types::Range::new(
            lsp_types::Position::new(0, 13),
            lsp_types::Position::new(0, 14),
        );
        assert_eq!(hover.and_then(|hover| hover.range), Some(chars));
    }
}
