use std::path::PathBuf;

use gpui::{Context, PathPromptOptions, Window};
use model::Command;

use super::ShellView;
use crate::focused_chat::focused_chat;
use crate::pane_spec::PaneSpec;

impl ShellView {
    pub(super) fn reopen_closed_tab(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.state.reopen_closed() {
            self.refresh(cx);
        }
    }

    pub(super) fn new_terminal(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let id = format!("t{}", self.state.serials().next_pane());
        let chat = focused_chat(self.state.layout());
        let spec = match (chat, self.data.active_project()) {
            (None, Some(project)) => PaneSpec::home_terminal(&id, &project),
            (chat, _) => PaneSpec::terminal(&id, chat.as_deref(), None),
        };
        self.open_spec(spec, cx);
    }

    pub(super) fn new_browser(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let id = format!("b{}", self.state.serials().next_pane());
        let state = self.homepage_state();
        let spec = PaneSpec::browser(&id).map(|spec| PaneSpec { state, ..spec });
        self.open_spec(spec, cx);
    }

    pub(super) fn new_file(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for("a new file") else {
            return;
        };
        let id = self.state.serials().next_pane();
        self.open_spec(PaneSpec::untitled(&chat, &id), cx);
    }

    pub(super) fn open_review(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for("a review") else {
            return;
        };
        self.open_spec(PaneSpec::review(&chat), cx);
    }

    pub(super) fn toggle_console(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let spec = match PaneSpec::console() {
            Ok(spec) => spec,
            Err(error) => return tracing::warn!(%error, "the console could not be described"),
        };
        let layout = self.state.layout();
        let open = layout.find_pane(&spec.type_id, &spec.params);
        match open.map(|pane| pane.pane_id.clone()) {
            Some(pane) => self.state.apply(vec![Command::ClosePane { pane }]),
            None => self.state.open_spec(spec),
        }
        self.refresh(cx);
    }

    pub(super) fn reveal_workspace_path(
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

    pub(super) fn open_workspace_file(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = self.chat_for("a file") else {
            return;
        };
        self.open_spec(PaneSpec::editor(&chat, path), cx);
    }

    pub(super) fn open_file_from_disk(
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

    pub(super) fn import_repo(
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

    fn open_host_file(
        &mut self,
        chat: &str,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.open_spec(PaneSpec::host_file(chat, path), cx);
    }

    pub(super) fn add_repo(
        &self,
        path: &str,
    ) {
        if let Err(error) = self.data.commands().import_repo(path) {
            tracing::warn!(%error, "the repository was not added");
        }
    }

    fn chat_for(
        &self,
        what: &str,
    ) -> Option<String> {
        let chat = focused_chat(self.state.layout());
        if chat.is_none() {
            tracing::info!("{what} needs a chat to be focused first");
        }
        chat
    }

    pub(super) fn open_spec(
        &mut self,
        spec: crate::error::Result<PaneSpec>,
        cx: &mut Context<'_, Self>,
    ) {
        match spec {
            Ok(spec) => {
                self.state.open_spec(spec);
                self.refresh(cx);
            },
            Err(error) => tracing::warn!(%error, "the pane could not be described"),
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
