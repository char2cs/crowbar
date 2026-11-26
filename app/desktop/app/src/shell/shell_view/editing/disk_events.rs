use gpui::Context;
use protocol::PaneId;
use ui::{ToastContent, ToastKind, ToastLifetime};

use crate::shell::shell_view::ShellView;

const CHANGED_ON_DISK: &str =
    "Your unsaved changes are kept. Saving will overwrite the on-disk version.";

impl ShellView {
    pub(crate) fn disk_changed(
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

    pub(crate) fn take_disk(
        &mut self,
        read: (&PaneId, (&str, editorview::SaveMark)),
        cx: &mut Context<'_, Self>,
    ) {
        let took = self.source.native().editors().take_disk(read, cx);
        self.disk_taken(read.0, took, cx);
    }

    pub(crate) fn disk_missing(
        &mut self,
        missing: (&PaneId, editor::OpenFailure, editorview::SaveMark),
        cx: &mut Context<'_, Self>,
    ) {
        if self.source.native().editors().disk_missing(missing, cx) {
            self.refresh(cx);
        }
    }

    pub(crate) fn disk_taken(
        &mut self,
        pane: &PaneId,
        took: editorview::DiskTake,
        cx: &mut Context<'_, Self>,
    ) {
        let editors = self.source.native().editors();
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
}

#[cfg(test)]
mod tests {
    use editorview::DiskTake;
    use gpui::TestAppContext;

    use crate::shell::shell_view::test_support::open_file;
    use crate::shell::shell_view::tests::open_shell;

    fn said_after(
        took: DiskTake,
        cx: &mut TestAppContext,
    ) -> usize {
        let mut opened = open_shell(cx);
        let pane = open_file(&mut opened, "src/pricing.ts");
        opened.shell.update(&mut opened.window, |shell, cx| {
            let before = shell.notices.read(cx).len();
            shell.disk_taken(&pane, took, cx);
            shell.notices.read(cx).len() - before
        })
    }

    #[gpui::test]
    fn only_edits_kept_over_a_changed_file_say_it_changed_on_disk(cx: &mut TestAppContext) {
        let cases = [
            (DiskTake::Taken, 0),
            (DiskTake::Kept, 0),
            (DiskTake::Conflicted, 1),
        ];
        for (took, said) in cases {
            assert_eq!(said_after(took, cx), said, "{took:?}");
        }
    }
}
