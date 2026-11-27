use gpui::{Context, Entity, Hsla};
use gpui_kit::component::input::{
    EditorState, RangeDecoration, RangeDecorationCollection, RangeDecorationStyle,
};
use ui::{Appearance, Theme};

use crate::Settings;
use crate::editor_view::EditorView;
use crate::editor_view::find::occurrences::word_query;

const DARK_WORD: u32 = 0x5757_57b8;
const LIGHT_WORD: u32 = 0x5757_5740;

pub(in crate::editor_view) fn collection_in(
    state: &Entity<EditorState>,
    cx: &mut Context<'_, EditorView>,
) -> RangeDecorationCollection {
    state.update(cx, |state, cx| {
        state.create_range_decorations_collection(Vec::new(), cx)
    })
}

impl EditorView {
    pub(in crate::editor_view) fn caret_moved(
        &mut self,
        _: Entity<EditorState>,
        cx: &mut Context<'_, Self>,
    ) {
        self.mark_occurrences(cx);
        cx.notify();
    }

    pub(in crate::editor_view) fn mark_occurrences(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let marks = self.occurrence_marks(cx).unwrap_or_default();
        let wanted = marks.iter().map(|mark| mark.range().clone());
        if !wanted.eq(self.word_marks.get_ranges(cx)) {
            self.word_marks.set(marks, cx);
        }
    }

    fn occurrence_marks(
        &self,
        cx: &Context<'_, Self>,
    ) -> Option<Vec<RangeDecoration>> {
        let state = self.state.read(cx);
        let caret = state.selected_range();
        if !Settings::of(cx).assists.occurrences || !caret.is_empty() {
            return None;
        }
        let text = state.value();
        let word = editor::word_at(&text, caret.start)?;
        let found = word_query(&text, &word).matches(&text).ok()?;
        let color = word_color(cx);
        let mark = |range| {
            RangeDecoration::new(range)
                .with_style(RangeDecorationStyle::Fill)
                .with_color(color)
        };
        Some(found.into_iter().map(mark).collect())
    }
}

fn word_color(cx: &Context<'_, EditorView>) -> Hsla {
    let rgba = match Theme::of(cx).appearance() {
        Appearance::Dark => DARK_WORD,
        Appearance::Light => LIGHT_WORD,
    };
    Hsla::from(gpui::rgba(rgba))
}

#[cfg(test)]
mod tests {
    use std::ops::Range;

    use gpui::TestAppContext;

    use crate::editor_view::test_support::{opened, press};
    use crate::{Access, Assists, Settings};

    #[gpui::test]
    fn the_caret_word_is_marked_wherever_it_occurs_only_with_highlight_occurrences_on(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("ab x ab abc", Some(1)), Access::Editable, cx);
        window.update(|window, cx| window.focus(&gpui::Focusable::focus_handle(&view, cx), cx));
        let marked = |window: &mut gpui::VisualTestContext| -> Vec<Range<usize>> {
            window.run_until_parked();
            view.read_with(window, |view, cx| view.word_marks.get_ranges(cx))
        };
        let assists = Assists {
            occurrences: true,
            ..Assists::default()
        };
        let on = Settings {
            assists,
            ..Settings::default()
        };
        window.update(|_, cx| on.install(cx));
        assert_eq!(marked(window), [0..2, 5..7], "on the word at the caret");
        press("right right right", window);
        assert_eq!(marked(window), vec![3..4], "follows the caret");
        window.update(|_, cx| Settings::default().install(cx));
        assert_eq!(marked(window), Vec::<Range<usize>>::new(), "off");
    }
}
