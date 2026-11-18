//! Copy, Cut and Paste of the Files panel and its keyboard.

use gpui::{Context, KeyDownEvent, Window};
use sidebar::{ClipMode, FileClipboard, FileKey};

use crate::file_search::opens_search;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn stage_file(
        &mut self,
        path: String,
        mode: ClipMode,
    ) {
        let root = self.data.workspace_root();
        self.tree.files.clipboard = Some(FileClipboard {
            root,
            paths: vec![path],
            mode,
        });
    }

    pub(crate) fn paste_files(
        &mut self,
        dir: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(clipboard) = self.tree.files.clipboard.clone() else {
            return;
        };
        let commands = clipboard.paste_into((&self.data.workspace_root(), dir));
        if commands.is_empty() {
            return;
        }
        for command in commands {
            self.send(command, cx);
        }
        if !clipboard.survives_paste() {
            self.tree.files.clipboard = None;
        }
    }

    pub(crate) fn files_key(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.inline.is_some() || self.search.is_some() {
            return;
        }
        let modifiers = event.keystroke.modifiers;
        let key = &event.keystroke.key;
        if opens_search(key, modifiers) {
            cx.stop_propagation();
            self.open_file_search(window, cx);
            return;
        }
        let secondary = modifiers.secondary();
        let parsed = FileKey::plain(key)
            .filter(|_| !secondary)
            .or(FileKey::with_modifier(key).filter(|_| secondary));
        let Some(key) = parsed else {
            return;
        };
        let rows = self.visible_files();
        let selected = self.tree.files.selected.as_deref();
        if let Some(intent) = key.step(&rows, selected) {
            cx.stop_propagation();
            self.apply(intent, window, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Intent;

    use crate::test_support::prelude::*;

    #[gpui::test]
    fn a_copy_pastes_into_a_directory_and_a_cut_is_consumed(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        let stage = |mode| Intent::StageFile {
            path: "README.md".into(),
            mode,
        };
        let paste = || Intent::PasteFiles { dir: "src".into() };
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(stage(ClipMode::Copy), window, cx);
            this.apply(paste(), window, cx);
            this.apply(paste(), window, cx);
            this.apply(stage(ClipMode::Cut), window, cx);
            this.apply(paste(), window, cx);
            this.apply(paste(), window, cx);
        });
        let copy = Command::CopyFile {
            path: "README.md".into(),
            destination: "src/README.md".into(),
        };
        let moved = Command::RenameFile {
            path: "README.md".into(),
            new_path: "src/README.md".into(),
        };
        assert_eq!(recorder.sent(), vec![copy.clone(), copy, moved]);
    }
}
