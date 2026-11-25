use gpui::{App, Context, Styled as _, Window, px};
use gpui_kit::base::input::{RopeExt as _, TabSize};
use gpui_kit::component::Sizable as _;
use gpui_kit::component::input::{Editor, EditorState, GutterStyle};

use super::EditorView;
use crate::{Access, Settings};

impl EditorView {
    pub(super) fn follow_settings(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let settings = Settings::of(cx);
        self.state
            .update(cx, |state, cx| apply(state, &settings, (window, cx)));
        cx.notify();
    }

    pub(super) fn editor(
        &self,
        window: &Window,
        cx: &App,
    ) -> Editor {
        let settings = Settings::of(cx);
        let lines = self.state.read(cx).text().lines_len();
        let mono = (
            gpui::font(ui::Theme::of(cx).mono_family()),
            px(settings.font_size),
        );
        let gap = super::gutter_gap::monaco_gap(lines, (&mono.0, mono.1), window);
        Editor::new(&self.state)
            .appearance(false)
            .xsmall()
            .readonly(self.access == Access::ReadOnly)
            .size_full()
            .font_family(ui::Theme::of(cx).mono_family())
            .text_size(px(settings.font_size))
            .line_height(settings.row_height())
            .gutter_style(GutterStyle::default().with_gap(gap))
    }
}

pub(super) fn apply(
    state: &mut EditorState,
    settings: &Settings,
    (window, cx): (&mut Window, &mut Context<'_, EditorState>),
) {
    let tab = TabSize {
        tab_size: settings.tab_size,
        hard_tabs: false,
    };
    state.set_tab_size(tab, cx);
    state.set_soft_wrap(settings.word_wrap == crate::WordWrap::On, window, cx);
    state.set_line_number(settings.line_numbers, window, cx);
    state.set_indent_guides(settings.indent_guides, window, cx);
    let every = settings.whitespace == editor::WhitespaceMode::All;
    state.set_show_whitespaces(every, window, cx);
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, Entity, Focusable as _, Pixels, TestAppContext, VisualTestContext, px};

    use super::super::test_support::{opened, press};
    use super::*;
    use crate::Access;

    const LONG_LINE_WORDS: usize = 400;

    fn caret(
        view: &Entity<EditorView>,
        window: &mut VisualTestContext,
    ) -> Option<(Bounds<Pixels>, Pixels)> {
        window.run_until_parked();
        window.read(|cx| view.read(cx).state.read(cx).cursor_layout())
    }

    #[gpui::test]
    fn settings_push_font_size_line_height_and_wrap_to_an_open_editor(cx: &mut TestAppContext) {
        let text = "word ".repeat(LONG_LINE_WORDS);
        let (view, window) = opened((&text, None), Access::Editable, cx);
        window.update(|window, cx| window.focus(&view.focus_handle(cx), cx));
        let (start, row) = caret(&view, window).unwrap_or_default();
        assert_eq!(row, px(20.0), "develop's 14px at 1.4, rounded up");
        let numbered_x = start.origin.x;
        let changed = Settings {
            font_size: 18.0,
            line_height: 1.5,
            tab_size: 4,
            word_wrap: crate::WordWrap::On,
            line_numbers: false,
            whitespace: editor::WhitespaceMode::Trailing,
            ..Settings::default()
        };
        window.update(|_, cx| changed.install(cx));
        let (start, row) = caret(&view, window).unwrap_or_default();
        assert_eq!(row, px(27.0), "18px at 1.5");
        assert!(start.origin.x < numbered_x, "the gutter lost its numbers");
        press("@input::MoveToEnd", window);
        let (end, _) = caret(&view, window).unwrap_or_default();
        assert!(end.origin.y > start.origin.y, "the long line wraps");
        press("tab", window);
        let typed = view.read_with(window, EditorView::text);
        assert!(typed.ends_with("word     "), "a tab types four spaces");
    }
}
