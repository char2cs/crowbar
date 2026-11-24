use std::rc::Rc;

use gpui::Context;

use super::EditorView;
use crate::LanguageServer;
use crate::lsp_completion::LspCompletion;
use crate::lsp_definition::{LspDefinition, elsewhere};
use crate::lsp_hover::LspHover;
use crate::lsp_link::LspLink;

impl EditorView {
    /// Tells `server` about the document this view shows, from now until the view goes, and
    /// asks it for hovers, completions and definitions, as develop's editor does for a
    /// workspace file.
    pub fn attach_language_server(
        &mut self,
        server: Rc<dyn LanguageServer>,
        cx: &mut Context<'_, Self>,
    ) {
        let link = LspLink::new(server, self.state.downgrade());
        let hover = LspHover {
            link: Rc::clone(&link),
        };
        let completion = LspCompletion {
            link: Rc::clone(&link),
        };
        let definition = LspDefinition {
            link: Rc::clone(&link),
        };
        let shows = show_elsewhere(cx.entity().downgrade(), Rc::clone(&link));
        self.state.update(cx, |state, _| {
            state.lsp_mut().hover_provider = Some(Rc::new(hover));
            state.lsp_mut().completion_provider = Some(Rc::new(completion));
            state.lsp_mut().definition_provider = Some(Rc::new(definition));
            state.lsp_mut().show_document = Some(shows);
        });
        cx.on_release(|view, cx| view.lsp_close(cx)).detach();
        self.lsp = Some(link);
        self.lsp_open(cx);
    }

    pub(super) fn lsp_open(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(link) = &self.lsp {
            link.open(self.toolbar.path(), cx);
        }
    }

    pub(super) fn lsp_changed(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(link) = &self.lsp {
            link.changed(cx);
        }
    }

    pub(super) fn lsp_saved(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(link) = &self.lsp {
            link.saved(cx);
        }
    }

    fn lsp_close(
        &self,
        cx: &mut gpui::App,
    ) {
        if let Some(link) = &self.lsp {
            link.close(cx);
        }
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

    use super::super::test_support::served;

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
        window.update(|_, cx| view.update(cx, |view, cx| view.rebind("src/b.ts", cx)));
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
