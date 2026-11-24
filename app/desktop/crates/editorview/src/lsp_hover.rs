//! Hover from the document's language server, as develop's Monaco hover provider asks it.

use std::rc::Rc;

use gpui::{App, AppContext as _, Task, Window};
use gpui_kit::base::input::{HoverProvider, Rope, RopeExt as _};
use serde_json::{Map, json};

use crate::lsp_link::LspLink;

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
        let position = text.offset_to_position(offset);
        let mut fields = Map::new();
        fields.insert("position".to_owned(), json!(position));
        let asked = self.link.request("hover", fields, cx);
        cx.background_spawn(async move {
            let answer = asked.await?;
            Ok(serde_json::from_value(answer).ok().flatten())
        })
    }
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
        window.simulate_input("x");
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
}
