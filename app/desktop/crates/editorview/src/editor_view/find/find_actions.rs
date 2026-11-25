use editor::{FindOption, Vertical};
use gpui::{
    AnyElement, ClickEvent, Context, Div, InteractiveElement as _, IntoElement as _,
    ParentElement as _, StatefulInteractiveElement as _, Styled as _, Window, div, px,
};
use gpui_kit::base::input::{Replace, Search};
use ui::{Icon, IconName, Surface, Theme, Tooltip};

use crate::Access;
use crate::chrome::find_widget::FindWidget;
use crate::editor_view::EditorView;
use crate::editor_view::reveal::Reveal;
use crate::keys::{
    CloseFind, FindNext, FindPrevious, ReplaceAll, ReplaceOne, ToggleFindInSelection,
    ToggleMatchCase, TogglePreserveCase, ToggleRegex, ToggleWholeWord,
};

const FIND_BUTTON_WIDTH: f32 = 36.0;
const FIND_BUTTON_HEIGHT: f32 = 32.0;
const FIND_ICON_SIZE: f32 = 16.0;
const FIND_BUTTON_RADIUS: f32 = 4.0;
const FIND_BUTTON_LABEL: &str = "Find in file";
pub(in crate::editor_view) const FIND_BUTTON: &str = "find-in-file";

impl EditorView {
    pub(in crate::editor_view) fn find_button(cx: &mut Context<'_, Self>) -> AnyElement {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let trigger = div()
            .id(FIND_BUTTON)
            .debug_selector(|| FIND_BUTTON.to_owned())
            .flex()
            .w(px(FIND_BUTTON_WIDTH))
            .h(px(FIND_BUTTON_HEIGHT))
            .items_center()
            .justify_center()
            .rounded(px(FIND_BUTTON_RADIUS))
            .hover(|style| style.bg(palette.accent()))
            .on_click(cx.listener(|view, _: &ClickEvent, window, cx| {
                view.open_find(Reveal::Find, window, cx);
            }))
            .child(
                Icon::new(IconName::MagnifyingGlass)
                    .size(px(FIND_ICON_SIZE))
                    .color(palette.muted_foreground()),
            );
        Tooltip::new(FIND_BUTTON, FIND_BUTTON_LABEL, trigger).into_any_element()
    }

    pub(in crate::editor_view) fn find_actions(
        element: Div,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        element
            .on_action(cx.listener(|view, _: &Search, window, cx| {
                view.open_find(Reveal::Find, window, cx);
            }))
            .on_action(cx.listener(|view, _: &Replace, window, cx| {
                view.open_find(Reveal::Replace, window, cx);
            }))
            .on_action(cx.listener(|view, _: &FindNext, window, cx| {
                view.find_step(Vertical::Down, window, cx);
            }))
            .on_action(cx.listener(|view, _: &FindPrevious, window, cx| {
                view.find_step(Vertical::Up, window, cx);
            }))
            .on_action(cx.listener(|view, _: &ToggleMatchCase, _, cx| {
                view.find_toggle(FindOption::MatchCase, cx);
            }))
            .on_action(cx.listener(|view, _: &ToggleWholeWord, _, cx| {
                view.find_toggle(FindOption::WholeWord, cx);
            }))
            .on_action(cx.listener(|view, _: &ToggleRegex, _, cx| {
                view.find_toggle(FindOption::Regex, cx);
            }))
            .on_action(cx.listener(|view, _: &TogglePreserveCase, _, cx| {
                view.find_toggle(FindOption::PreserveCase, cx);
            }))
            .on_action(cx.listener(|view, _: &ToggleFindInSelection, _, cx| {
                view.find.update(cx, FindWidget::toggle_scope);
            }))
            .on_action(cx.listener(|view, _: &ReplaceOne, window, cx| {
                view.find
                    .update(cx, |find, cx| find.replace_one(window, cx));
            }))
            .on_action(cx.listener(|view, _: &ReplaceAll, window, cx| {
                view.find
                    .update(cx, |find, cx| find.replace_all(window, cx));
            }))
            .on_action(cx.listener(Self::close_find))
    }

    pub(in crate::editor_view) fn open_find(
        &mut self,
        reveal: Reveal,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let reveal = match self.access {
            Access::ReadOnly => Reveal::Find,
            Access::Editable => reveal,
        };
        self.find
            .update(cx, |find, cx| find.open(reveal, window, cx));
    }

    fn find_step(
        &mut self,
        toward: Vertical,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.find.read(cx).is_shown() {
            self.open_find(Reveal::Find, window, cx);
        }
        self.find.update(cx, |find, cx| find.step(toward, cx));
    }

    fn find_toggle(
        &mut self,
        option: FindOption,
        cx: &mut Context<'_, Self>,
    ) {
        self.find.update(cx, |find, cx| find.toggle(option, cx));
    }

    fn close_find(
        &mut self,
        _: &CloseFind,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.find.read(cx).is_shown() {
            return cx.propagate();
        }
        self.find.update(cx, |find, cx| find.close(window, cx));
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, VisualTestContext};

    use crate::editor_view::test_support::{find_of, opened, press, selected_of};
    use crate::{Access, EditorView};

    struct Seen {
        shown: bool,
        label: String,
        selected: std::ops::Range<usize>,
    }

    fn seen(
        view: &gpui::Entity<EditorView>,
        window: &mut VisualTestContext,
    ) -> Seen {
        let find = find_of(view, window);
        let (shown, label) = window.read(|cx| (find.read(cx).is_shown(), find.read(cx).label(cx)));
        let selected = selected_of(view, window);
        Seen {
            shown,
            label,
            selected,
        }
    }

    #[gpui::test]
    fn ctrl_f_finds_the_caret_word_and_enter_and_shift_enter_walk_its_matches(
        cx: &mut TestAppContext
    ) {
        let text = "total x Total\ntotal";
        let cases = [
            ("@input::Search", true, "1 of 3", 0..0),
            ("@input::Search enter", true, "1 of 3", 0..5),
            ("@input::Search enter enter", true, "2 of 3", 8..13),
            (
                "@input::Search enter enter enter enter",
                true,
                "1 of 3",
                0..5,
            ),
            ("@input::Search shift-enter", true, "3 of 3", 14..19),
            ("f3 f3", true, "2 of 3", 8..13),
            (
                "@input::Search @editorview::ToggleMatchCase",
                true,
                "1 of 2",
                0..0,
            ),
            (
                "@input::Search @editorview::ToggleMatchCase enter enter",
                true,
                "2 of 2",
                14..19,
            ),
            ("@input::Search escape", false, "1 of 3", 0..0),
        ];
        for (keys, shown, label, selected) in cases {
            let (view, window) = opened((text, Some(1)), Access::Editable, cx);
            press(keys, window);
            let seen = seen(&view, window);
            assert_eq!(seen.shown, shown, "{keys}");
            assert_eq!(seen.label, label, "{keys}");
            assert_eq!(seen.selected, selected, "{keys}");
        }
    }

    #[gpui::test]
    fn typing_a_query_moves_to_its_first_match_and_a_bad_one_finds_nothing(
        cx: &mut TestAppContext
    ) {
        let cases = [
            ("@input::Search", "x", "", "1 of 1", 6..7),
            ("@input::Search", "t.tal", "", "No results", 0..1),
            (
                "@input::Search",
                "t.tal",
                "@editorview::ToggleRegex",
                "1 of 3",
                0..1,
            ),
            (
                "@input::Search",
                "(",
                "@editorview::ToggleRegex",
                "No results",
                0..0,
            ),
            (
                "@input::Search",
                "tal",
                "@editorview::ToggleWholeWord",
                "No results",
                2..5,
            ),
        ];
        for (open, query, keys, label, selected) in cases {
            let (view, window) = opened(("total x Total\ntotal", Some(1)), Access::Editable, cx);
            press(open, window);
            window.simulate_input(query);
            press(keys, window);
            let seen = seen(&view, window);
            assert_eq!(seen.label, label, "{query} {keys}");
            assert_eq!(seen.selected, selected, "{query} {keys}");
        }
    }

    #[gpui::test]
    fn ctrl_h_replaces_one_match_or_all_of_them_in_one_undo_step(cx: &mut TestAppContext) {
        let cases = [
            ("enter", "total x Total\ntotal"),
            ("enter enter", "sum x Total\ntotal"),
            ("enter enter enter", "sum x sum\ntotal"),
            ("@editorview::ReplaceAll", "sum x sum\nsum"),
            (
                "@editorview::ReplaceAll escape @input::Undo",
                "total x Total\ntotal",
            ),
        ];
        for (keys, expected) in cases {
            let (view, window) = opened(("total x Total\ntotal", Some(1)), Access::Editable, cx);
            press("@input::Replace", window);
            window.simulate_input("sum");
            press(keys, window);
            assert_eq!(view.read_with(window, EditorView::text), expected, "{keys}");
        }
    }

    #[gpui::test]
    fn a_read_only_view_finds_but_does_not_open_replace(cx: &mut TestAppContext) {
        let (view, window) = opened(("total total", Some(1)), Access::ReadOnly, cx);
        press("@input::Replace", window);
        window.simulate_input("x");
        assert_eq!(view.read_with(window, EditorView::text), "total total");
        assert_eq!(seen(&view, window).label, "No results");
    }
}
