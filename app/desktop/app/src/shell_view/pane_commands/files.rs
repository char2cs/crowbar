use std::path::PathBuf;

use gpui::{ClipboardItem, Context, Window};
use ideshell::PathAction;

use super::ShellView;
use crate::pane_spec::PaneSpec;

const A_FILE: &str = "a file";

// SPEC: docs/triage/2026-11-18-side-by-side-audit/16-dlg.md DLG-046: rfd's GTK chooser, several files.
const OPEN_FILES: ui::FileChoice = ui::FileChoice {
    title: "Open File",
    kind: ui::ChoiceKind::Files,
    filter: None,
};

impl ShellView {
    pub(in crate::shell_view) fn reveal_workspace_path(
        &self,
        relative: &str,
    ) {
        let Some(root) = self.data.workspace_root() else {
            return note_reveal_needs_workspace();
        };
        let Some(target) = crate::reveal_target::reveal_target(&root, relative) else {
            return warn_reveal_left_workspace(relative);
        };
        if let Err(error) = platform::reveal_in_file_manager(&target) {
            warn_file_manager_unopened(&error);
        }
    }

    pub(in crate::shell_view) fn act_on_path(
        &mut self,
        action: PathAction,
        relative: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if PathAction::is_virtual(relative) {
            return Self::act_on_virtual_path(action, relative, cx);
        }
        match action {
            PathAction::CopyRelativePath => {
                cx.write_to_clipboard(ClipboardItem::new_string(relative.to_owned()));
            },
            PathAction::CopyPath => self.copy_absolute(relative, cx),
            PathAction::Reveal => self.reveal_workspace_path(relative),
            PathAction::OpenInTerminal => {
                let folder = relative.rsplit_once('/').map_or("", |(folder, _)| folder);
                self.open_terminal_at(folder, cx);
            },
            _ => {},
        }
    }

    fn act_on_virtual_path(
        action: PathAction,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let copies = matches!(action, PathAction::CopyPath | PathAction::CopyRelativePath);
        if copies {
            cx.write_to_clipboard(ClipboardItem::new_string(path.to_owned()));
        }
    }

    fn copy_absolute(
        &self,
        relative: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(root) = self.data.workspace_root() else {
            tracing::info!("copying a full path needs a workspace on this computer");
            return;
        };
        let full = root.join(relative).to_string_lossy().into_owned();
        cx.write_to_clipboard(ClipboardItem::new_string(full));
    }

    pub(in crate::shell_view) fn open_workspace_file(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for(A_FILE) else {
            return;
        };
        self.open_spec(PaneSpec::editor(&chat, path), cx);
    }

    pub(in crate::shell_view) fn preview_workspace_file(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for(A_FILE) else {
            return;
        };
        match PaneSpec::editor(&chat, path) {
            Ok(spec) => self.state.preview_spec(spec),
            Err(error) => tracing::warn!(%error, "the file could not be described"),
        }
        self.refresh(cx);
    }

    pub(in crate::shell_view) fn open_file_from_disk(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for(A_FILE) else {
            return;
        };
        let picked = ui::FileChooser::choose(OPEN_FILES, window, cx);
        cx.spawn_in(window, async move |this, cx| {
            let paths = picked.await;
            this.update(cx, |this, cx| this.open_host_files(&chat, &paths, cx))
                .ok()
        })
        .detach();
    }

    fn open_host_files(
        &mut self,
        chat: &str,
        paths: &[PathBuf],
        cx: &mut Context<'_, Self>,
    ) {
        for path in paths {
            self.open_spec(PaneSpec::host_file(chat, &path.to_string_lossy()), cx);
        }
    }
}

fn note_reveal_needs_workspace() {
    tracing::info!("revealing needs a workspace on this computer");
}

fn warn_reveal_left_workspace(relative: &str) {
    tracing::warn!(path = relative, "a reveal path left the workspace");
}

fn warn_file_manager_unopened(error: &platform::Error) {
    tracing::warn!(%error, "the file manager could not be opened");
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::core_panes::EDITOR;
    use crate::shell_view::test_support::panes_of;
    use crate::shell_view::tests::open_shell;
    use crate::test_support::answer_choices_with;

    #[gpui::test]
    fn open_file_asks_develop_s_chooser_and_opens_every_picked_file(cx: &mut TestAppContext) {
        let asked = answer_choices_with(cx, &["/a/one.md", "/a/two.md"]);
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        shell.update_in(&mut opened.window, ShellView::open_file_from_disk);
        opened.window.run_until_parked();
        assert_eq!(*asked.borrow(), [OPEN_FILES]);
        let editors = shell.read_with(&opened.window, |shell, _| panes_of(shell, EDITOR).len());
        assert_eq!(editors, 2, "one editor per picked file");
    }
}
