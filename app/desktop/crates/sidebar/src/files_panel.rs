//! The Files panel.

use std::rc::Rc;

use gpui::{AnyElement, Context, InteractiveElement, IntoElement, ParentElement, Styled, div, px};
use ui::{ContextMenu, ScrollArea};

use crate::expansion::Expansion;
use crate::file_kind::FileKind;
use crate::file_menu::file_menu;
use crate::file_tree_entry::FileTreeEntry;
use crate::files_context::FilesContext;
use crate::git_status::GitStatus;
use crate::lock::Lock;
use crate::menu_scope::MenuScope;
use crate::panel_message::panel_message;
use crate::row_input::RowInput;
use crate::scale::SPACE_XS;
use crate::shell_answers::ShellAnswers;
use crate::sidebar::Sidebar;

const ROOT_STRIP: f32 = 48.0;
const NO_MATCH: &str = "No matching files";

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
            field: edit.field.clone(),
            error: edit.error.clone(),
        })
    }

    pub(crate) fn files_panel(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        if self.data.files().is_empty() {
            return panel_message("No folder open", cx);
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
            selected: self.tree.files.selected.clone(),
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
            .children(self.file_search_box())
            .child(div().flex_1().min_h_0().child(body))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use crate::sidebar::Sidebar;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_files_panel_renders_the_workspace_files(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        assert!(!sidebar.read_with(cx, |this, _| this.data.files().is_empty()));
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
            this.inline.as_ref().map(|edit| edit.field.clone())
        });
        let Some(field) = field else {
            return false;
        };
        let handle = field.read_with(cx, gpui::Focusable::focus_handle);
        cx.update(|window, _| handle.is_focused(window))
    }

    #[gpui::test]
    fn choosing_new_file_from_the_menu_leaves_the_input_focused(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
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
