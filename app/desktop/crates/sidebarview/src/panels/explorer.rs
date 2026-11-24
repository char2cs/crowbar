//! The Files panel.

mod file_dialog_view;
mod file_drop;
mod file_menu;
mod file_paste;
mod file_tree_entry;
mod file_tree_row;
mod files_context;
mod menu_lines;
mod search;

pub(crate) use file_tree_entry::FileTreeEntry;
pub use file_tree_row::FileGlyph;
pub(crate) use file_tree_row::badges;
pub(crate) use file_tree_row::{TreeLook, frame, glyph, tree_guides};
pub(crate) use files_context::neighbours;
pub(crate) use search::FileSearch;

use std::rc::Rc;

use gpui::{
    AnyElement, App, Context, InteractiveElement, IntoElement, ParentElement, Styled, Window, div,
    px,
};
use sidebar::{Expansion, FileKind, FolderPhase, GitStatus, Lock, MenuScope, ShellAnswers};
use ui::{ContextMenu, ScrollArea};

use crate::panels::chats::RowInput;
use crate::panels::chats::Trailing;
use crate::panels::explorer::file_menu::file_menu;
use crate::panels::explorer::files_context::FilesContext;
use crate::panels::message::panel_message;
use crate::sidebar::SPACE_XS;
use crate::sidebar::Sidebar;

const ROOT_STRIP: f32 = 48.0;
const NO_MATCH: &str = "No matching files";

fn empty_message(phase: FolderPhase) -> &'static str {
    match phase {
        FolderPhase::NoFolder => "No folder open",
        FolderPhase::Loading => "Loading files…",
        FolderPhase::Listed => "Folder is empty",
    }
}

fn root_entry() -> FileTreeEntry {
    FileTreeEntry {
        path: String::new(),
        name: String::new(),
        depth: 0,
        kind: FileKind::Directory(Expansion::Expanded),
        status: GitStatus::Clean,
        counts: None,
    }
}

impl Sidebar {
    pub(crate) fn menu_scope(&self) -> MenuScope {
        let mut lock = Lock::Unlocked;
        if self.data.files_locked() {
            lock = Lock::Locked;
        }
        MenuScope {
            root: self.data.workspace_root(),
            lock,
            local: self.data.files_are_local(),
            can_generate_image: self.data.can_generate_image(),
            answers: ShellAnswers::of(&self.data),
        }
    }

    fn row_input(&self) -> Option<RowInput> {
        let edit = self.inline.as_ref()?;
        Some(RowInput {
            field: edit.input.clone(),
            error: edit.error.clone(),
            trailing: Trailing::Hidden,
        })
    }

    pub(crate) fn files_panel(
        &mut self,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let phase = self.data.folder_phase();
        if phase == FolderPhase::NoFolder || self.data.files().is_empty() {
            return panel_message(empty_message(phase), cx);
        }
        let files = self.visible_files();
        if files.is_empty() {
            return self.panel_frame(panel_message(NO_MATCH, cx), cx);
        }
        let scope = self.menu_scope();
        let handler = Self::row_handler(cx);
        let root_menu = file_menu(&root_entry(), &scope, &handler);
        let context = FilesContext {
            handler,
            scope: Rc::new(scope),
            editing: self.tree.editing().clone(),
            input: self.row_input(),
            selected: self.highlighted(window, cx),
            look: TreeLook::of(
                &self.data.file_tree_view(),
                self.search.as_ref().map(|search| search.query.as_str()),
            ),
        };
        let strip = ContextMenu::new("files-root-menu", div().h(px(ROOT_STRIP)), root_menu);
        let list = div()
            .flex()
            .flex_col()
            .py(SPACE_XS)
            .children(context.rows(files))
            .child(strip);
        let scroll = ScrollArea::new("files-scroll", &self.scrolls.files, list);
        self.panel_frame(scroll.into_any_element(), cx)
    }

    fn highlighted(
        &self,
        window: &Window,
        cx: &App,
    ) -> Option<String> {
        let focused = self.files_focus.contains_focused(window, cx);
        let selected = self.tree.files().selected().filter(|_| focused);
        selected.map(str::to_owned)
    }

    fn panel_frame(
        &self,
        body: AnyElement,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        div()
            .id("files-panel")
            .flex()
            .flex_col()
            .size_full()
            .track_focus(&self.files_focus)
            .on_key_down(cx.listener(Self::files_key))
            .on_drop(file_drop::on_drop(&Self::row_handler(cx), String::new()))
            .children(self.file_search_box(cx))
            .child(div().flex_1().min_h_0().child(body))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use sidebar::FolderPhase;

    use crate::sidebar::Sidebar;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_files_panel_renders_the_workspace_files(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        assert!(!sidebar.read_with(cx, |this, _| this.data.files().is_empty()));
    }

    #[test]
    fn an_empty_panel_says_whether_a_folder_is_open_loading_or_empty() {
        let said = [
            FolderPhase::NoFolder,
            FolderPhase::Loading,
            FolderPhase::Listed,
        ]
        .map(super::empty_message);
        assert_eq!(
            said,
            ["No folder open", "Loading files…", "Folder is empty"]
        );
    }

    fn started(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> bool {
        sidebar.read_with(cx, |this, _| this.inline.is_some())
    }

    fn choose_first_item_at(
        y: f32,
        cx: &mut gpui::VisualTestContext,
    ) {
        let at = point(px(120.0), px(y));
        cx.simulate_mouse_down(at, gpui::MouseButton::Right, gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_keystrokes("down enter");
    }

    fn input_is_focused(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> bool {
        let field = sidebar.read_with(cx, |this, _| {
            this.inline.as_ref().map(|edit| edit.input.clone())
        });
        let Some(field) = field else {
            return false;
        };
        let handle = field.read_with(cx, gpui::Focusable::focus_handle);
        cx.update(|window, _| handle.is_focused(window))
    }

    #[gpui::test]
    fn choosing_new_file_from_the_menu_leaves_the_input_focused(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        let mut step = 0_u8;
        while !started(&sidebar, cx) && step < 100 {
            choose_first_item_at(4.0 + 6.0 * f32::from(step), cx);
            step += 1;
        }
        assert!(started(&sidebar, cx), "no Files row opened an inline input");
        cx.run_until_parked();
        assert!(input_is_focused(&sidebar, cx));
    }
}
