//! Opening, closing and drawing the Files panel's filter box.

use gpui::{AnyElement, AppContext as _, Context, IntoElement, ParentElement, Styled, Window, div};
use ui::{IconName, Surface, TextInput, TextInputEvent, TextInputStyle};

use crate::file_row::FileRow;
use crate::file_search::{FileSearch, matching};
use crate::scale::SPACE_SM;
use crate::sidebar::Sidebar;

const PLACEHOLDER: &str = "Search";

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
        let rows = self.data.files();
        match &self.search {
            Some(search) => matching(rows, &search.query),
            None => rows,
        }
    }

    pub(crate) fn file_search_box(&self) -> Option<AnyElement> {
        let search = self.search.as_ref()?;
        Some(
            div()
                .flex_none()
                .p(SPACE_SM)
                .child(search.field.clone())
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
