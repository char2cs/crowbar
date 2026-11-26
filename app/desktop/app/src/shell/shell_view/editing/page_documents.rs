//! A file pane's page (its rich view or its preview) over the text the pane's editor owns.

use gpui::{Context, Window};
use protocol::{PaneId, ShellFrame};

use crate::panes::editor::document_request::DocumentRequest;
use crate::panes::editor::editor_panes::PaneView;
use crate::panes::kinds::pane_spec::PaneSpec;
use crate::shell::inbox::request_queue::send;
use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn serve_page_documents(
        &self,
        mounted: &[PaneId],
        documents: Vec<DocumentRequest>,
        cx: &Context<'_, Self>,
    ) {
        for pane in mounted {
            self.send_page_text(pane, cx);
        }
        for request in documents {
            send(&self.requests, ShellRequest::Document(request));
        }
    }

    pub(crate) fn send_page_text(
        &self,
        pane: &PaneId,
        cx: &Context<'_, Self>,
    ) {
        let Some(text) = self.source.native().editors().page_text(pane, cx) else {
            return;
        };
        if let Err(error) = self
            .state
            .host()
            .send(pane, &ShellFrame::DocumentText { text })
        {
            tracing::warn!(%error, %pane, "a file pane's page could not be sent its text");
        }
    }

    pub(crate) fn send_preview_texts(
        &self,
        pane: &PaneId,
        cx: &Context<'_, Self>,
    ) {
        for preview in self.source.native().editors().previews_of(pane) {
            self.send_page_text(&preview, cx);
        }
    }

    pub(crate) fn take_page_text(
        &self,
        (pane, text): (&PaneId, &str),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let editors = self.source.native().editors();
        editors.take_page_text((pane, text), window, cx);
        self.send_preview_texts(pane, cx);
    }

    pub(crate) fn show_pane_view(
        &mut self,
        pane: &PaneId,
        view: PaneView,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.source.native().editors().show(pane, view) {
            return;
        }
        self.state
            .apply(vec![layout::Command::FocusPane { pane: pane.clone() }]);
        self.refresh(cx);
    }

    pub(crate) fn open_file_preview(
        &mut self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        let Some((chat, file, preview)) = self.source.native().editors().preview_of(pane) else {
            return;
        };
        let focus = layout::Command::FocusPane { pane: pane.clone() };
        self.state.apply(vec![focus]);
        self.open_spec(PaneSpec::preview(&chat, (&file, preview)), cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::panes::kinds::core_panes::EDITOR;
    use crate::panes::kinds::pane_spec::PaneSpec;
    use crate::shell::shell_view::test_support::panes_of;
    use crate::shell::shell_view::tests::open_shell;

    #[gpui::test]
    fn the_preview_button_opens_the_file_s_preview_beside_its_editor(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        let views = shell.update_in(&mut opened.window, |shell, _, cx| {
            shell.open_spec(PaneSpec::editor("t1", "README.md"), cx);
            let file = panes_of(shell, EDITOR).remove(0);
            shell.open_file_preview(&file, cx);
            let layout = shell.state.layout();
            let view_of = |pane: &protocol::PaneId| {
                let instance = layout.pane(pane)?;
                instance.params.get("view").map(str::to_owned)
            };
            panes_of(shell, EDITOR)
                .iter()
                .map(view_of)
                .collect::<Vec<_>>()
        });
        assert_eq!(views, [None, Some("preview".to_owned())]);
    }
}
