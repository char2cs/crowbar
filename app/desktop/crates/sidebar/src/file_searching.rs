//! Opening, closing and drawing the Files panel's filter box.

use gpui::{
    AnyElement, AppContext as _, Context, InteractiveElement, IntoElement, ParentElement, Pixels,
    Point, Styled, Window, div,
};
use ui::{IconName, Surface, TextInput, TextInputEvent, TextInputStyle};

use crate::file_row::FileRow;
use crate::file_search::{FileSearch, matching, step_match};
use crate::filter_button::{ClickHandler, FilterButton};
use crate::filter_menu::filter_menu;
use crate::match_direction::MatchDirection;
use crate::scale::{SPACE_SM, SPACE_XS};
use crate::sidebar::Sidebar;

const PLACEHOLDER: &str = "Search";

fn filter_click(sidebar: gpui::Entity<Sidebar>) -> ClickHandler {
    Box::new(move |event, window, cx| {
        let at = event.position();
        sidebar.update(cx, |this, cx| this.open_filter_menu(at, window, cx));
    })
}

fn steps_back(keystroke: &gpui::Keystroke) -> bool {
    keystroke.key == "enter" && keystroke.modifiers.shift
}

impl Sidebar {
    pub(crate) fn open_file_search(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let field = cx.new(|cx| {
            TextInput::new(PLACEHOLDER, cx)
                .on(Surface::Chrome)
                .style(TextInputStyle::Compact)
                .leading_icon(IconName::MagnifyingGlass)
        });
        field.read(cx).focus(window);
        let subscription = cx.subscribe_in(
            &field,
            window,
            |this, _, event: &TextInputEvent, window, cx| match event {
                TextInputEvent::Changed(text) => this.set_file_query(text, cx),
                TextInputEvent::Submitted => this.step_file_match(MatchDirection::Forward, cx),
                TextInputEvent::Cancelled => this.close_file_search(window, cx),
                _ => {},
            },
        );
        self.search = Some(FileSearch {
            field,
            query: String::new(),
            _subscription: subscription,
        });
        cx.notify();
    }

    pub(crate) fn close_file_search(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.search = None;
        window.focus(&self.files_focus);
        cx.notify();
    }

    fn set_file_query(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(search) = self.search.as_mut() {
            text.clone_into(&mut search.query);
        }
        cx.notify();
    }

    pub(crate) fn visible_files(&self) -> Vec<FileRow> {
        let view = self.data.file_tree_view();
        let rows = view.visible(self.data.files());
        let rows = match &self.search {
            Some(search) => matching(rows, &search.query),
            None => rows,
        };
        if view.compact_folders {
            return FileRow::compacted(&rows);
        }
        rows
    }

    fn search_key(
        &mut self,
        event: &gpui::KeyDownEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if !steps_back(&event.keystroke) {
            return;
        }
        cx.stop_propagation();
        self.step_file_match(MatchDirection::Backward, cx);
    }

    pub(crate) fn step_file_match(
        &mut self,
        direction: MatchDirection,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(search) = self.search.as_ref() else {
            return;
        };
        let rows = self.visible_files();
        let selected = self.tree.files.selected.as_deref();
        let next = step_match(&rows, (&search.query, selected), direction);
        if let Some(path) = next {
            self.tree.files.selected = Some(path);
            cx.notify();
        }
    }

    pub(crate) fn open_filter_menu(
        &mut self,
        at: Point<Pixels>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let entries = filter_menu(&self.data.file_tree_view());
        self.open_menu((entries, at), window, cx);
        if let Some(menu) = self.menu.as_mut() {
            menu.filter = true;
        }
    }

    pub(crate) fn file_search_box(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let search = self.search.as_ref()?;
        let sidebar = cx.entity();
        let button = FilterButton {
            active: self.data.file_tree_view().any_filter_off(),
            on_click: filter_click(sidebar),
        };
        let backwards = cx.listener(|this, event: &gpui::KeyDownEvent, _, cx| {
            this.search_key(event, cx);
        });
        Some(
            div()
                .flex()
                .flex_none()
                .gap(SPACE_XS)
                .p(SPACE_SM)
                .capture_key_down(backwards)
                .child(div().flex_1().min_w_0().child(search.field.clone()))
                .child(button)
                .into_any_element(),
        )
    }
}

#[cfg(test)]
mod tests {
    use crate::sidebar::Sidebar;
    use crate::test_support::prelude::*;

    fn names(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> Vec<String> {
        sidebar.read_with(cx, |this, _| {
            this.visible_files()
                .into_iter()
                .map(|row| row.name)
                .collect()
        })
    }

    #[gpui::test]
    fn typing_in_the_box_filters_the_files_and_escape_restores_them(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        let all = names(&sidebar, cx);
        sidebar.update_in(cx, Sidebar::open_file_search);
        cx.simulate_input("lib");
        cx.run_until_parked();
        assert_eq!(names(&sidebar, cx), ["src", "lib.rs"]);
        cx.simulate_keystrokes("escape");
        cx.run_until_parked();
        assert_eq!(names(&sidebar, cx), all);
    }

    #[gpui::test]
    fn slash_in_the_files_panel_opens_the_box(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        sidebar.update_in(cx, |this, window, _| window.focus(&this.files_focus));
        cx.simulate_keystrokes("/");
        cx.run_until_parked();
        assert!(sidebar.read_with(cx, |this, _| this.search.is_some()));
    }
}
