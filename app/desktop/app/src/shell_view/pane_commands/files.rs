use std::path::PathBuf;

use gpui::{ClipboardItem, Context, PathPromptOptions, Window};
use layout_view::PathAction;

use super::ShellView;
use crate::pane_spec::PaneSpec;

impl ShellView {
    pub(in crate::shell_view) fn reveal_workspace_path(
        &self,
        relative: &str,
    ) {
        let Some(root) = self.data.workspace_root() else {
            tracing::info!("revealing needs a workspace on this computer");
            return;
        };
        let Some(target) = crate::reveal_target::reveal_target(&root, relative) else {
            tracing::warn!(path = relative, "a reveal path left the workspace");
            return;
        };
        if let Err(error) = shell_platform::reveal_in_file_manager(&target) {
            tracing::warn!(%error, "the file manager could not be opened");
        }
    }

    pub(in crate::shell_view) fn act_on_path(
        &mut self,
        action: PathAction,
        relative: &str,
        cx: &mut Context<'_, Self>,
    ) {
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
        let Some(chat) = self.chat_for("a file") else {
            return;
        };
        self.open_spec(PaneSpec::editor(&chat, path), cx);
    }

    pub(in crate::shell_view) fn open_file_from_disk(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for("a file") else {
            return;
        };
        let picked = cx.prompt_for_paths(PathPromptOptions {
            files: true,
            directories: false,
            multiple: false,
            prompt: Some("Open".into()),
        });
        cx.spawn_in(window, async move |this, cx| {
            let path = match picked.await {
                Ok(Ok(Some(paths))) => first_path(&paths),
                _ => None,
            };
            match path {
                Some(path) => this
                    .update(cx, |this, cx| this.open_host_file(&chat, &path, cx))
                    .ok(),
                None => None,
            }
        })
        .detach();
    }

    pub(in crate::shell_view) fn import_repo(
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let picked = cx.prompt_for_paths(PathPromptOptions {
            files: false,
            directories: true,
            multiple: false,
            prompt: Some("Add".into()),
        });
        cx.spawn_in(window, async move |this, cx| {
            let path = match picked.await {
                Ok(Ok(Some(paths))) => first_path(&paths),
                _ => None,
            };
            match path {
                Some(path) => this.update(cx, |this, _| this.add_repo(&path)).ok(),
                None => None,
            }
        })
        .detach();
    }

    pub(in crate::shell_view) fn open_dropped(
        &mut self,
        paths: &[protocol::FsPath],
        cx: &mut Context<'_, Self>,
    ) {
        if paths.is_empty() {
            return;
        }
        let Some(chat) = self.chat_for("a dropped file") else {
            return;
        };
        for path in paths {
            self.open_host_file(&chat, path.as_str(), cx);
        }
    }

    fn open_host_file(
        &mut self,
        chat: &str,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.open_spec(PaneSpec::host_file(chat, path), cx);
    }

    pub(in crate::shell_view) fn add_repo(
        &self,
        path: &str,
    ) {
        if let Err(error) = self.data.commands().import_repo(path) {
            tracing::warn!(%error, "the repository was not added");
        }
    }
}

fn first_path(paths: &[PathBuf]) -> Option<String> {
    paths
        .first()
        .map(|path| path.to_string_lossy().into_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_first_picked_path_is_the_one_opened() {
        let picked = [PathBuf::from("/a/one"), PathBuf::from("/a/two")];
        assert_eq!(first_path(&picked).as_deref(), Some("/a/one"));
    }

    #[test]
    fn an_empty_pick_opens_nothing() {
        assert_eq!(first_path(&[]), None);
    }
}
