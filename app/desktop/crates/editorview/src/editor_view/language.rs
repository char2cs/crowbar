use std::rc::Rc;

use gpui::Context;

use super::EditorView;
use crate::lsp::lsp_completion::LspCompletion;
use crate::lsp::lsp_definition::{LspDefinition, elsewhere};
use crate::lsp::lsp_hover::LspHover;
use crate::lsp::lsp_link::LspLink;

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
        let shows = show_elsewhere(cx.entity().downgrade(), link);
        self.state.update(cx, |state, _| {
            state.lsp_mut().hover_provider = Some(Rc::new(hover));
            state.lsp_mut().completion_provider = Some(Rc::new(completion));
            state.lsp_mut().definition_provider = Some(Rc::new(definition));
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
