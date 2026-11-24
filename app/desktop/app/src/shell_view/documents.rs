use gpui::{Context, Window};
use protocol::{DisplayText, PaneId};

use ui::{ToastContent, ToastKind, ToastLifetime};

use super::ShellView;

const CHANGED_ON_DISK: &str =
    "Your unsaved changes are kept. Saving will overwrite the on-disk version.";
use crate::document_request::DocumentRequest;
use crate::editor_panes::{Landing, PaneView};
use crate::request_queue::send;
use crate::shell_request::ShellRequest;

impl ShellView {
    pub(super) fn on_document(
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
            DocumentRequest::Dirty { pane, dirty: false } => {
                self.source.native().editors().discard_draft(&pane);
                self.state.clear_dirty(&pane);
                self.dirty_changed(cx);
                self.send_preview_texts(&pane, cx);
            },
            DocumentRequest::Save { pane } => self.save_document(&pane, window, cx),
            DocumentRequest::SavedAs { pane, saved } => self.adopt_saved_as(&pane, &saved, cx),
            DocumentRequest::Paused { pane } => self.edits_paused(&pane, cx),
            DocumentRequest::Saved { pane, text } => {
                self.source.native().editors().saved(&pane, &text, cx);
            },
            DocumentRequest::SaveFailed { pane, reason } => self.refuse_save(&pane, reason, cx),
            DocumentRequest::DiskRead { pane, text, since } => {
                self.take_disk((&pane, (&text, since)), window, cx);
            },
            DocumentRequest::PageEdited { pane, text } => {
                self.take_page_text((&pane, &text), window, cx);
            },
            DocumentRequest::ShowSource { pane } => {
                self.show_pane_view(&pane, PaneView::Source, cx);
            },
            DocumentRequest::ShowRich { pane } => self.show_pane_view(&pane, PaneView::Rich, cx),
            DocumentRequest::Preview { pane } => self.open_file_preview(&pane, cx),
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

    pub(super) fn disk_changed(
        &self,
        outcome: store::Outcome,
        cx: &Context<'_, Self>,
    ) {
        let editors = self.source.native().editors();
        match outcome {
            store::Outcome::FileChanged { workspace, path } => {
                editors.disk_changed((&workspace, Some(&path)), cx);
            },
            store::Outcome::FilesSynced { workspace } => {
                editors.disk_changed((&workspace, None), cx);
            },
            _ => {},
        }
    }

    fn take_disk(
        &mut self,
        read: (&PaneId, (&str, editorview::SaveMark)),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let pane = read.0;
        let editors = self.source.native().editors();
        let took = editors.take_disk(read, window, cx);
        if took == editorview::DiskTake::Taken {
            self.send_page_text(pane, cx);
            self.send_preview_texts(pane, cx);
        }
        if took != editorview::DiskTake::Conflicted {
            return;
        }
        let Some(name) = editors.file_name(pane) else {
            return;
        };
        let said = ToastContent::new(ToastKind::Warning, CHANGED_ON_DISK)
            .titled(format!("{name} changed on disk"));
        self.show_keyed(&format!("disk:{pane}"), (said, ToastLifetime::Auto), cx);
    }

    fn edits_paused(
        &mut self,
        pane: &PaneId,
        cx: &Context<'_, Self>,
    ) {
        self.source.native().editors().keep_draft(pane, cx);
        self.send_preview_texts(pane, cx);
        if self.autosaves() {
            self.source.native().editors().autosave(pane, cx);
        }
    }

    pub(super) fn land_document(
        &mut self,
        (pane, read): (&PaneId, Result<String, editor::OpenFailure>),
        cx: &mut Context<'_, Self>,
    ) {
        match self.source.native().editors().land(pane, read) {
            Landing::Shown => self.state.settle_file_pane(pane),
            Landing::Refused(failure) => return self.refuse_read(pane, &failure, cx),
            Landing::Quiet => {},
        }
        self.layout.update(cx, |_, cx| cx.notify());
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

    pub(super) fn refuse_save(
        &mut self,
        pane: &PaneId,
        reason: String,
        cx: &mut Context<'_, Self>,
    ) {
        let (title, message) = crate::save_refusal::words(reason);
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
    use crate::shell_view::tests::{Opened, open_shell};

    const PATH: &str = "src/pricing.ts";

    fn opened_file(opened: &mut Opened) -> PaneId {
        opened.shell.update(&mut opened.window, |shell, cx| {
            shell.open_workspace_file(PATH, cx);
            let layout = shell.state.layout();
            let focused = layout.active_tab().and_then(layout::Tab::focused);
            focused.cloned().expect("the file pane has focus")
        })
    }

    fn toasts(opened: &mut Opened) -> usize {
        opened.window.executor().advance_clock(ui::timing::SLOW);
        opened.window.run_until_parked();
        opened
            .shell
            .update(&mut opened.window, |shell, cx| shell.notices.read(cx).len())
    }

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
            let mut opened = open_shell(cx);
            let pane = opened_file(&mut opened);
            let before = toasts(&mut opened);
            land(&mut opened, &pane, read);
            assert_eq!(toasts(&mut opened) - before, said, "toasts");
            assert_eq!(pane_state(&mut opened, &pane), expected);
        }
    }
}
