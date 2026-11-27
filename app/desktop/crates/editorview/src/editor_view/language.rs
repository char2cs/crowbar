use std::rc::Rc;

use gpui::Context;

use super::EditorView;
use crate::lsp::lsp_completion::LspCompletion;
use crate::lsp::lsp_definition::{LspDefinition, elsewhere};
use crate::lsp::lsp_hover::LspHover;
use crate::lsp::lsp_link::LspLink;
use crate::lsp::lsp_semantic_tokens::LspSemanticTokens;

impl EditorView {
    pub(super) fn ask_language_server(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(link) = self.document.read(cx).lsp() else {
            return;
        };
        let hover = LspHover {
            link: Rc::clone(&link),
        };
        let completion = LspCompletion {
            link: Rc::clone(&link),
        };
        let definition = LspDefinition {
            link: Rc::clone(&link),
        };
        let tokens = LspSemanticTokens {
            link: Rc::clone(&link),
        };
        let shows = show_elsewhere(cx.entity().downgrade(), link);
        self.state.update(cx, |state, _| {
            state.lsp_mut().hover_provider = Some(Rc::new(hover));
            state.lsp_mut().completion_provider = Some(Rc::new(completion));
            state.lsp_mut().definition_provider = Some(Rc::new(definition));
            state.lsp_mut().semantic_tokens_provider = Some(Rc::new(tokens));
            state.lsp_mut().show_document = Some(shows);
        });
    }
}

fn show_elsewhere(
    view: gpui::WeakEntity<EditorView>,
    link: Rc<LspLink>,
) -> gpui_kit::base::input::ShowDocumentHandler {
    Rc::new(move |shown, _, cx| tell_elsewhere((&view, &link), shown, cx))
}

fn tell_elsewhere(
    (view, link): (&gpui::WeakEntity<EditorView>, &LspLink),
    shown: &lsp_types::ShowDocumentParams,
    cx: &mut gpui::App,
) -> bool {
    let Some(event) = elsewhere(shown, link.path().as_deref()) else {
        return false;
    };
    view.update(cx, |_, cx| cx.emit(event)).is_ok()
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::TestAppContext;
    use serde_json::json;

    use super::super::test_support::{document_of, served};
    use crate::{Assists, Highlighting, Settings};

    fn assisting(
        assists: Assists,
        window: &mut gpui::VisualTestContext,
    ) {
        let settings = Settings {
            assists,
            ..Settings::default()
        };
        window.update(|_, cx| settings.install(cx));
        window.run_until_parked();
    }

    #[gpui::test]
    fn suggestions_open_while_typing_only_with_auto_completion_on(cx: &mut TestAppContext) {
        let (view, window, _) = served(("src/a.ts", "a"), cx);
        let off = Assists {
            completion: false,
            ..Assists::default()
        };
        for assists in [Assists::default(), off] {
            assisting(assists, window);
            assert_eq!(opens_on_a_dot(&view, window), assists.completion);
        }
    }

    fn opens_on_a_dot(
        view: &gpui::Entity<super::EditorView>,
        window: &mut gpui::VisualTestContext,
    ) -> bool {
        let state = window.read(|cx| view.read(cx).state.clone());
        let provider = window.read(|cx| state.read(cx).lsp().completion_provider.clone());
        window.update(|_, cx| {
            provider.is_some_and(|provider| provider.is_completion_trigger(0, ".", cx))
        })
    }

    fn tokens_of(
        view: &gpui::Entity<super::EditorView>,
        window: &mut gpui::VisualTestContext,
    ) -> Vec<u32> {
        let asked = window.update(|window, cx| {
            let state = view.read(cx).state.clone();
            let text = state.read(cx).text().clone();
            let provider = state.read(cx).lsp().semantic_tokens_provider.clone()?;
            Some(provider.semantic_tokens(&text, 0..text.len(), window, cx))
        });
        let Some(asked) = asked else {
            return Vec::new();
        };
        let answer = std::rc::Rc::new(std::cell::RefCell::new(None));
        let slot = std::rc::Rc::clone(&answer);
        window
            .spawn(async move |_| *slot.borrow_mut() = asked.await.ok())
            .detach();
        window.run_until_parked();
        let tokens = answer.take().unwrap_or_default().data;
        tokens.iter().map(|token| token.token_type).collect()
    }

    #[gpui::test]
    fn the_server_s_tokens_colour_the_text_only_with_semantic_highlighting_on(
        cx: &mut TestAppContext
    ) {
        let (view, window, server) = served(("src/a.ts", "const a = 1"), cx);
        let data = [0, 0, 5, 8, 0, 0, 6, 1, 8, 1];
        server.answer("semanticTokens", json!({ "data": data }));
        assert_eq!(tokens_of(&view, window), [8, 10], "readonly is a constant");
        let off = Assists {
            highlighting: Highlighting::Syntax,
            ..Assists::default()
        };
        assisting(off, window);
        assert_eq!(tokens_of(&view, window), Vec::<u32>::new());
    }

    #[gpui::test]
    fn a_view_opens_its_file_sends_its_text_once_edits_pause_and_reopens_it_when_rebound(
        cx: &mut TestAppContext
    ) {
        let (view, window, server) = served(("src/a.ts", "a"), cx);
        window.simulate_input(" ");
        window.executor().advance_clock(Duration::from_millis(399));
        assert_eq!(server.verbs(), ["didOpen"], "still typing");
        window.executor().advance_clock(Duration::from_millis(1));
        window.run_until_parked();
        let document = document_of(&view, window);
        document.update(window, |document, cx| document.rebind("src/b.ts", cx));
        assert_eq!(
            server.verbs(),
            ["didOpen", "didChange", "didClose", "didOpen"]
        );
        assert_eq!(
            server.bodies()[0],
            json!({"path": "src/a.ts", "languageId": "typescript", "text": "a"})
        );
        assert_eq!(
            server.bodies()[1],
            json!({"path": "src/a.ts", "text": " a"})
        );
    }
}
