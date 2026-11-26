use gpui::{Context, Window};
use protocol::{DisplayText, PaneId};

use ui::{ToastContent, ToastKind, ToastLifetime};

use crate::shell::shell_view::ShellView;

use crate::panes::editor::document_request::DocumentRequest;
use crate::panes::editor::editor_panes::{Landing, PaneView};
use crate::panes::editor::save_refusal;
use crate::shell::inbox::request_queue::send;
use crate::shell::inbox::shell_request::ShellRequest;

impl ShellView {
    pub(crate) fn on_document(
        &mut self,
        request: DocumentRequest,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match request {
            DocumentRequest::Read { pane, read } => self.read_landed((&pane, read), window, cx),
            DocumentRequest::Retry { pane } => self.retry_document(&pane, cx),
            DocumentRequest::Dirty { pane, dirty: true } => {
                self.state.mark_dirty(pane);
                self.dirty_changed(cx);
            },
            DocumentRequest::Dirty { pane, dirty: false } => self.turned_clean(&pane, cx),
            DocumentRequest::Save { pane } => self.save_document(&pane, window, cx),
            DocumentRequest::SavedAs { pane, saved } => {
                self.adopt_saved_as((&pane, &saved), cx);
            },
            DocumentRequest::Paused { pane } => self.edits_paused(&pane, cx),
            DocumentRequest::Saved { pane, text } => {
                self.source.native().editors().saved(&pane, &text, cx);
            },
            DocumentRequest::SaveFailed { pane, reason } => self.refuse_save(&pane, reason, cx),
            DocumentRequest::DiskRead { pane, text, since } => {
                self.take_disk((&pane, (&text, since)), cx);
            },
            DocumentRequest::Reloaded { pane, text, since } => {
                self.take_reloaded((&pane, (&text, since)), cx);
            },
            DocumentRequest::DiskMissing {
                pane,
                failure,
                since,
            } => self.disk_missing((&pane, failure, since), cx),
            DocumentRequest::PageEdited { pane, text } => {
                self.take_page_text((&pane, &text), window, cx);
            },
            other => self.on_view_request(other, window, cx),
        }
    }

    fn on_view_request(
        &mut self,
        request: DocumentRequest,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match request {
            DocumentRequest::ShowSource { pane } => {
                self.show_pane_view(&pane, PaneView::Source, cx);
            },
            DocumentRequest::ShowRich { pane } => self.show_pane_view(&pane, PaneView::Rich, cx),
            DocumentRequest::Preview { pane } => self.open_file_preview(&pane, cx),
            DocumentRequest::Definition { pane, path, at } => {
                self.open_definition((&pane, &path, at), window, cx);
            },
            DocumentRequest::DefinitionOutside { path, .. } => self.definition_outside(path, cx),
            _ => {},
        }
    }

    fn read_landed(
        &mut self,
        (pane, read): (&PaneId, Result<String, editor::OpenFailure>),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.land_document((pane, read), cx);
        if self.source.native().editors().prepare(pane, window, cx) {
            self.refresh(cx);
        }
        self.send_page_text(pane, cx);
        self.send_preview_texts(pane, cx);
    }

    fn turned_clean(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        self.source.native().editors().discard_draft(pane);
        self.state.clear_dirty(pane);
        self.dirty_changed(cx);
        self.send_preview_texts(pane, cx);
    }

    fn edits_paused(
        &mut self,
        pane: &PaneId,
        cx: &Context<'_, Self>,
    ) {
        if !self.source.native().editors().leads(pane, cx) {
            return;
        }
        self.source.native().editors().keep_draft(pane, cx);
        self.send_preview_texts(pane, cx);
        if self.autosaves() {
            self.source.native().editors().autosave(pane, cx);
        }
    }

    pub(crate) fn land_document(
        &mut self,
        (pane, read): (&PaneId, Result<String, editor::OpenFailure>),
        cx: &mut Context<'_, Self>,
    ) {
        match self.source.native().editors().land(pane, read) {
            Landing::Shown => self.state.settle_file_pane(pane),
            Landing::Refused(failure) => return self.refuse_read(pane, &failure, cx),
            Landing::Quiet => {},
        }
        self.refresh(cx);
    }

    fn retry_document(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        self.source.native().editors().retry(pane);
        self.layout.update(cx, |_, cx| cx.notify());
    }

    fn dirty_changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        send(&self.requests, ShellRequest::ResumeClosing);
        self.refresh(cx);
    }

    pub(crate) fn refuse_save(
        &mut self,
        pane: &PaneId,
        reason: String,
        cx: &mut Context<'_, Self>,
    ) {
        let (title, message) = save_refusal::words(reason);
        let said = ToastContent::new(ToastKind::Error, message).titled(title.to_owned());
        self.show_keyed(&format!("save:{pane}"), (said, ToastLifetime::Auto), cx);
    }

    fn refuse_read(
        &mut self,
        pane: &PaneId,
        failure: &editor::OpenFailure,
        cx: &mut Context<'_, Self>,
    ) {
        let said = DisplayText::new(failure.title()).and_then(|title| {
            DisplayText::new(failure.message()).map(|message| (Some(title), message))
        });
        match said {
            Ok(said) => self.refuse_open(pane, &said, cx),
            Err(error) => {
                tracing::warn!(%error, %pane, "a refused read has no words to say why");
                self.state.withdraw_open(pane);
                self.refresh(cx);
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::panes::kinds::pane_spec::PaneSpec;
    use crate::shell::shell_view::test_support::{open_file, settled_notice_count};
    use crate::shell::shell_view::tests::{Opened, PLACED, open_started_shell, views_set};

    const PATH: &str = "src/pricing.ts";
    const README: &str = "README.md";

    fn pane_state(
        opened: &mut Opened,
        pane: &PaneId,
    ) -> Option<serde_json::Value> {
        opened.shell.update(&mut opened.window, |shell, _| {
            let instance = shell.state.layout().pane(pane);
            instance.map(|instance| instance.state.as_value().clone())
        })
    }

    fn land(
        opened: &mut Opened,
        pane: &PaneId,
        read: Result<String, editor::OpenFailure>,
    ) {
        let reading = (pane, read);
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.land_document(reading, cx);
        });
    }

    #[gpui::test]
    fn a_fresh_file_that_reads_is_kept_and_one_that_cannot_closes_and_says_why(
        cx: &mut TestAppContext
    ) {
        let refused = editor::OpenFailure::of(PATH, 413, None);
        let kept = serde_json::json!({"path": PATH});
        let cases = [(Ok("a".to_owned()), Some(kept), 0), (Err(refused), None, 1)];
        for (read, expected, said) in cases {
            let mut opened = open_started_shell(cx);
            let pane = open_file(&mut opened, PATH);
            let before = settled_notice_count(&mut opened);
            land(&mut opened, &pane, read);
            assert_eq!(settled_notice_count(&mut opened) - before, said, "toasts");
            assert_eq!(pane_state(&mut opened, &pane), expected);
        }
    }

    fn restored_readme(opened: &mut Opened) -> PaneId {
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.pane_area.placement = Some((webview::ParentWindow::X11(9), PLACED));
            let chat = shell.chat_for("a file").expect("a chat to read from");
            let mut spec = PaneSpec::editor(&chat, README).expect("a file spec");
            spec.state = protocol::PaneState::new(serde_json::json!({"path": README}))
                .expect("a restored file's state");
            shell.open_spec(Ok(spec), cx);
            let focused = shell.state.showing().and_then(layout::Tab::focused);
            focused.cloned().expect("the file pane has focus")
        })
    }

    #[gpui::test]
    fn a_markdown_file_that_comes_back_on_try_again_shows_its_rich_page(cx: &mut TestAppContext) {
        let mut opened = open_started_shell(cx);
        let pane = restored_readme(&mut opened);
        let gone = editor::OpenFailure::of(README, 404, None);
        land(&mut opened, &pane, Err(gone));
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.retry_document(&pane, cx);
        });
        let before = views_set(&opened.mock, webview::Visibility::Shown);
        land(&mut opened, &pane, Ok("# Title\n".to_owned()));
        assert!(
            views_set(&opened.mock, webview::Visibility::Shown) > before,
            "the rich page is shown"
        );
    }
}
