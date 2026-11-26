//! The Files panel.

mod copy_flash;
mod file_dialog_view;
mod file_drop;
mod file_menu;
mod file_paste;
mod file_tree_entry;
mod file_tree_row;
mod files_context;
mod menu_lines;
mod search;
mod sticky_ancestors;

pub(crate) use file_tree_entry::FileTreeEntry;
pub use file_tree_row::FileGlyph;
pub(crate) use file_tree_row::badges;
pub(crate) use file_tree_row::{TreeLook, frame, glyph, tree_guides};
pub(crate) use files_context::neighbours;
pub(crate) use search::FileSearch;

use std::rc::Rc;

use gpui::{
    AnyElement, App, ClickEvent, Context, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use sidebar::{
    ClipMode, Expansion, FileKind, FolderPhase, GitStatus, Lock, MenuScope, ShellAnswers,
};
use ui::{ContextMenu, ScrollArea};

use crate::panels::chats::RowInput;
use crate::panels::chats::Trailing;
use crate::panels::explorer::file_menu::file_menu;
use crate::panels::explorer::file_tree_row::FileTreeRow;
use crate::panels::explorer::files_context::FilesContext;
use crate::panels::explorer::sticky_ancestors::top_row;
use crate::panels::message::panel_message;
use crate::sidebar::SPACE_XS;
use crate::sidebar::Sidebar;

const ROOT_STRIP: f32 = 48.0;
const NO_MATCH: &str = "No matching files";
// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/styles/file-explorer-tree.css
const STACK_RULE_ALPHA: f32 = 0.406;

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

    /// Names the file of the focused pane's active editor tab, or `None` without one: the
    /// tree's keyboard cursor moves there when it changes.
    pub fn set_active_file(
        &mut self,
        path: Option<String>,
        cx: &mut Context<'_, Self>,
    ) {
        if self.active_file == path {
            return;
        }
        self.tree.select_file(path.clone());
        self.active_file = path;
        cx.notify();
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
        let context = self.files_context(window, cx);
        let root_menu = file_menu(&root_entry(), &context.scope, &context.handler);
        let strip = Self::root_strip(root_menu, cx);
        let rows = context.rows(files);
        let stack = self.sticky_stack(&context, &rows, cx);
        let list = div()
            .flex()
            .flex_col()
            .min_h_full()
            .py(SPACE_XS)
            .children(rows)
            .child(strip);
        let scroll = ScrollArea::new("files-scroll", &self.scrolls.files, list);
        let body = div().relative().size_full().child(scroll).children(stack);
        self.panel_frame(body.into_any_element(), cx)
    }

    fn files_context(
        &self,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) -> FilesContext {
        FilesContext {
            handler: Self::row_handler(cx),
            scope: Rc::new(self.menu_scope()),
            editing: self.tree.editing().clone(),
            input: self.row_input(),
            selected: self.highlighted(window, cx),
            cut: self.cut_paths(),
            copied: self.copied.clone(),
            look: TreeLook::of(
                &self.data.file_tree_view(),
                self.search.as_ref().map(|search| search.query.as_str()),
            ),
        }
    }

    // SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/components/file-explorer-sticky-ancestors.tsx
    fn sticky_stack(
        &self,
        context: &FilesContext,
        rows: &[FileTreeRow],
        cx: &App,
    ) -> Option<gpui::Div> {
        let scrolled = -f32::from(self.scrolls.files.offset().y) - f32::from(SPACE_XS);
        let top = top_row(scrolled, (context.look.height, rows.len()));
        let stuck = context.stuck(rows, top);
        if stuck.is_empty() {
            return None;
        }
        let palette = ui::Theme::of(cx).palette(ui::Surface::Chrome);
        let stack = div()
            .absolute()
            .top_0()
            .left_0()
            .right_0()
            .debug_selector(|| "sticky-ancestors".into());
        let rule = palette.border().opacity(STACK_RULE_ALPHA);
        Some(
            stack
                .bg(palette.background())
                .border_b_1()
                .border_color(rule)
                .children(stuck),
        )
    }

    // SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/hooks/use-tree-container-events.ts
    fn root_strip(
        root_menu: Vec<ui::MenuItem>,
        cx: &mut Context<'_, Self>,
    ) -> ContextMenu {
        let clear = cx.listener(|this, _: &ClickEvent, _, cx| {
            this.tree.select_file(None);
            cx.notify();
        });
        let blank = div().id("files-blank").absolute().inset_0().on_click(clear);
        ContextMenu::new("files-root-menu", blank, root_menu)
            .flex_1()
            .min_h(px(ROOT_STRIP))
    }

    fn cut_paths(&self) -> Vec<String> {
        let staged = self.tree.files().clipboard();
        let root = self.data.workspace_root();
        let cut = staged.filter(|staged| staged.mode == ClipMode::Cut && staged.root == root);
        cut.map_or_else(Vec::new, |staged| staged.paths.clone())
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
    use sidebar::{ClipMode, FolderPhase};

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

    fn highlighted(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> Option<String> {
        cx.update(|window, cx| sidebar.read(cx).highlighted(window, cx))
    }

    #[gpui::test]
    fn the_active_editor_tabs_file_takes_the_cursor_but_no_fill_while_the_tree_is_unfocused(
        cx: &mut TestAppContext
    ) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        sidebar.update(cx, |this, cx| {
            this.set_active_file(Some("src/lib.rs".into()), cx);
        });
        cx.run_until_parked();
        let cursor = sidebar.read_with(cx, |this, _| {
            this.tree.files().selected().map(str::to_owned)
        });
        assert_eq!(cursor.as_deref(), Some("src/lib.rs"));
        assert_eq!(highlighted(&sidebar, cx), None);
    }

    fn stage(mode: ClipMode) -> Intent {
        Intent::StageFile {
            path: "README.md".into(),
            mode,
        }
    }

    #[gpui::test]
    fn only_files_staged_by_cut_in_this_workspace_are_drawn_cut(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let cases = [(ClipMode::Cut, vec!["README.md"]), (ClipMode::Copy, vec![])];
        for (mode, want) in cases {
            sidebar.update_in(cx, |this, window, cx| this.apply(stage(mode), window, cx));
            let cut = sidebar.read_with(cx, |this, _| this.cut_paths());
            assert_eq!(cut, want, "{mode:?}");
        }
    }

    const FAR_BELOW_THE_ROWS: f32 = 700.0;

    fn right_click_at(
        y: f32,
        cx: &mut gpui::VisualTestContext,
    ) {
        let at = point(px(120.0), px(y));
        cx.simulate_mouse_down(at, gpui::MouseButton::Right, gpui::Modifiers::none());
        cx.simulate_mouse_up(at, gpui::MouseButton::Right, gpui::Modifiers::none());
        cx.run_until_parked();
    }

    fn files_open(
        cx: &mut TestAppContext,
        answer: Option<ui::MenuPath>,
    ) -> (Popped, gpui::Entity<Sidebar>, &mut gpui::VisualTestContext) {
        let popped = answer_menus_with(cx, answer);
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        (popped, sidebar, cx)
    }

    #[gpui::test]
    fn a_right_click_on_the_files_panel_asks_for_develops_native_menu(cx: &mut TestAppContext) {
        let (popped, _, cx) = files_open(cx, None);
        right_click_at(FAR_BELOW_THE_ROWS, cx);
        let popped = popped.borrow();
        let [request] = popped.as_slice() else {
            panic!("one native menu, got {popped:?}");
        };
        assert_eq!(
            menu_labels(&request.entries),
            [
                "New File",
                "New Folder",
                "Upload File",
                "Refresh",
                "Open All Files",
                "Collapse All",
                "Open in Terminal",
                "-",
                "Copy Path",
                "Copy Relative Path",
                "Reveal in Finder",
                "Copy",
                "Cut",
            ]
        );
        assert_eq!(request.at, point(px(120.0), px(FAR_BELOW_THE_ROWS)));
        assert_eq!(
            cx.debug_bounds("ui-menu"),
            None,
            "no menu drawn in the window"
        );
    }

    #[gpui::test]
    fn choosing_new_file_in_the_native_menu_opens_the_inline_input(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = files_open(cx, Some(vec![0]));
        right_click_at(FAR_BELOW_THE_ROWS, cx);
        assert!(started(&sidebar, cx), "New File opened no input");
    }

    #[gpui::test]
    fn a_click_on_the_empty_space_under_the_tree_clears_the_selection(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        sidebar.update(cx, |this, _| {
            this.tree.select_file(Some("README.md".into()));
        });
        cx.run_until_parked();
        cx.simulate_click(
            point(px(120.0), px(FAR_BELOW_THE_ROWS)),
            gpui::Modifiers::none(),
        );
        let selected = sidebar.read_with(cx, |this, _| {
            this.tree.files().selected().map(str::to_owned)
        });
        assert_eq!(selected, None);
    }

    #[gpui::test]
    fn the_empty_space_at_the_foot_of_the_panel_opens_the_root_menu(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        choose_first_item_at(FAR_BELOW_THE_ROWS, cx);
        assert!(
            started(&sidebar, cx),
            "the root menu's New File opened no input"
        );
    }
}
