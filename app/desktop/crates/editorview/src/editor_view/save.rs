use gpui::{Context, Window};
use gpui_kit::base::input::Rope;
use serde_json::{Map, Value, json};

use crate::SaveFormat;
use crate::editor_view::EditorView;
use crate::editor_view::replay::replace_keeping_selections as replaced;
use crate::keys::save_key::Save;
use crate::lsp::lsp_formatting::formatted;
use crate::{Access, EditorEvent, LspError, Settings};

impl EditorView {
    pub(super) fn save(
        &mut self,
        _: &Save,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let settings = Settings::of(cx);
        let formats =
            settings.save_format == SaveFormat::Formatted && self.access == Access::Editable;
        let Some(link) = self.document.read(cx).lsp().filter(|_| formats) else {
            return cx.emit(EditorEvent::SaveRequested);
        };
        let options = json!({ "tabSize": settings.tab_size, "insertSpaces": true });
        let mut fields = Map::new();
        fields.insert("options".to_owned(), options);
        let asked = link.request("formatting", fields, cx);
        let sent = self.state.read(cx).text().clone();
        cx.spawn_in(window, async move |view, cx| {
            let formatting = (sent, asked.await);
            view.update_in(cx, saving(formatting))
        })
        .detach();
    }

    fn save_formatted(
        &self,
        (sent, answer): Formatting,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let edits = answer
            .inspect_err(|error| tracing::warn!(%error, "the document could not be formatted"))
            .ok()
            .filter(|_| *self.state.read(cx).text() == sent);
        if let Some(text) = edits.and_then(|edits| formatted(&sent, edits)) {
            self.state
                .update(cx, |state, cx| replaced(state, &text, window, cx));
        }
        cx.emit(EditorEvent::SaveRequested);
    }
}

type Formatting = (Rope, Result<Value, LspError>);

fn saving(
    formatting: Formatting
) -> impl FnOnce(&mut EditorView, &mut Window, &mut Context<'_, EditorView>) {
    |view, window, cx| view.save_formatted(formatting, window, cx)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use serde_json::{Value, json};

    use crate::editor_view::EditorView;
    use crate::editor_view::test_support::{FakeServer, listened, press, served};
    use crate::{EditorEvent, SaveFormat, Settings};

    fn space(
        from: u32,
        to: u32,
    ) -> Value {
        let at = |character| lsp_types::Position::new(0, character);
        let range = lsp_types::Range::new(at(from), at(to));
        json!(lsp_types::TextEdit::new(range, " ".to_owned()))
    }

    fn saved_with(
        settings: Settings,
        cx: &mut TestAppContext,
    ) -> (String, usize, FakeServer) {
        let (view, window, server) = served(("src/a.ts", "a  =  1\n"), cx);
        server.answer("formatting", json!([space(1, 3), space(4, 6)]));
        window.update(|_, cx| settings.install(cx));
        let heard = listened(&view, window);
        press("@editorview::Save", window);
        window.run_until_parked();
        let text = view.read_with(window, EditorView::text);
        let heard = heard.borrow();
        let saves = heard
            .iter()
            .filter(|event| **event == EditorEvent::SaveRequested);
        (text, saves.count(), server)
    }

    #[gpui::test]
    fn a_manual_save_formats_the_document_first_only_with_format_on_save_on(
        cx: &mut TestAppContext
    ) {
        let formatting = Settings {
            save_format: SaveFormat::Formatted,
            ..Settings::default()
        };
        let cases = [(formatting, "a = 1\n"), (Settings::default(), "a  =  1\n")];
        for (settings, expected) in cases {
            let (text, saves, server) = saved_with(settings, cx);
            let on = settings.save_format == SaveFormat::Formatted;
            assert_eq!(text, expected, "format on save {on}");
            assert_eq!(saves, 1, "saved once");
            assert_eq!(server.verbs().contains(&"formatting"), on);
        }
    }
}
